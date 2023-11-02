package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/flags"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
	gaxv2 "github.com/googleapis/gax-go/v2"
)

type LockState struct {
	Name     string
	Token    string
	State    int32     // 1 = locked, > 1 = extended, 0 = available|released
	Start    time.Time // should be Spanner time, not local
	Duration int64     // seconds
	Error    error
}

type rsmT struct {
	muLock sync.RWMutex
	lock   map[string]*LockState // lock

	muSet sync.RWMutex
	set   map[string]map[string]struct{} // set
}

// Apply applies cmd to its internal state machine.
//
// GetLock:
// The cmd format is </name token durationInSec>.
// The return value is the lock state.
//
// ReleaseLock:
// The cmd format is <\name token>.
// The return value is the lock state.
//
// AddToSet:
// The cmd format is <+key "value" [/path/to/file]>.
// The return value is the total number of items
// under the input key.
func (s *rsmT) Apply(cmd string) int {
	switch {
	case strings.HasPrefix(cmd, "/"): // acquire lock
		ss := strings.Split(cmd[1:], " ")
		if len(ss) != 4 {
			return -1 // TODO: handle this properly
		}

		name := ss[0]
		token := ss[1]
		start, _ := time.Parse(time.RFC3339, ss[2])
		d, _ := strconv.ParseInt(ss[3], 10, 64)

		var ret int
		s.muLock.Lock()
		if _, ok := s.lock[name]; !ok {
			ret = 1
			s.lock[name] = &LockState{
				Name:     name,
				Token:    token,
				State:    1, // locked
				Start:    start,
				Duration: d,
			}
		} else {
			glog.Errorf("lock %v not available", name)
		}

		s.muLock.Unlock()
		return ret
	case strings.HasPrefix(cmd, "\\"): // release lock
		ss := strings.Split(cmd[1:], " ")
		if len(ss) != 2 {
			return -1 // TODO: handle this properly
		}

		name := ss[0]
		token := ss[1]

		var ret int
		s.muLock.Lock()
		if p, ok := s.lock[name]; ok {
			if token == p.Token {
				delete(s.lock, name)
			}
		} else {
			ret = -1
		}

		s.muLock.Unlock()
		return ret
	case strings.HasPrefix(cmd, "+"): // add to set
		ss := strings.Split(cmd[1:], " ")
		if len(ss) < 2 {
			return -1 // error
		}

		var path string
		key := ss[0]
		val := strings.Join(ss[1:len(ss)-1], " ")
		if len(ss) == 2 {
			val = ss[1]
		}

		if len(ss) > 2 {
			path = ss[len(ss)-1]
		}

		s.muSet.Lock()
		if _, ok := s.set[key]; !ok {
			s.set[key] = make(map[string]struct{})
		}

		s.set[key][val] = struct{}{}
		n := len(s.set[key])
		s.muSet.Unlock()

		func() {
			// See if we have a valid snapshot.
			if path == "" || !strings.HasPrefix(path, "_path:") {
				return
			}

			object := strings.Split(path, ":")[1]
			data, err := internal.GetSnapshot(object)
			if err != nil {
				glog.Errorf("GetSnapshot failed: %v", err)
				return
			}

			var copy map[string]map[string]struct{}
			err = json.Unmarshal(data, &copy)
			if err != nil {
				glog.Errorf("Unmarshal failed: %v", err)
				return
			}

			s.muSet.Lock()
			s.set = copy
			s.muSet.Unlock()
		}()

		return n
	default:
		return -1 // error
	}
}

func (s *rsmT) GetLockState(name string) LockState {
	var copy LockState
	s.muLock.Lock()
	v, ok := s.lock[name]
	if ok {
		copy = *v
		copy.Name = name
	}

	s.muLock.Unlock()
	return copy
}

// Members return the current members of the input key.
func (s *rsmT) Members(key string) []string {
	m := []string{}
	s.muSet.Lock()
	for k := range s.set[key] {
		m = append(m, k)
	}

	s.muSet.Unlock()
	return m
}

// Delete deletes the map under the input key.
func (s *rsmT) Delete(key string) {
	s.muSet.Lock()
	defer s.muSet.Unlock()
	delete(s.set, key)
}

// Clone returns a copy of the internal set.
func (s *rsmT) Clone() map[string]map[string]struct{} {
	copy := make(map[string]map[string]struct{})
	s.muSet.Lock()
	for k, v := range s.set {
		copy[k] = v
	}

	s.muSet.Unlock()
	return copy
}

// Reset resets the underlying map.
func (s *rsmT) Reset() {
	s.muSet.Lock()
	defer s.muSet.Unlock()
	s.set = map[string]map[string]struct{}{}
}

// NewRsm returns an instance of our replicated state machine set.
func NewRsm() *rsmT {
	return &rsmT{
		lock: map[string]*LockState{},
		set:  map[string]map[string]struct{}{},
	}
}

// BuildRsm builds the state machine to it's current state from our replicated log.
func BuildRsm(ctx context.Context, fd *FleetData, noBroadcast bool) error {
	defer func(begin time.Time) {
		atomic.StoreInt32(&fd.SetValueReady, 1)
		glog.Infof("fn:BuildRsm took %v", time.Since(begin))
	}(time.Now())

	paused := make(chan struct{}, 1)
	if !noBroadcast {
		nctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go broadcastRebuildRsmHeartbeat(nctx, fd, paused)
		<-paused // wait for the signal that others have acknowledged pause
	}

	limit := 20
	retries := -1
	var end bool
	for {
		retries++
		if retries >= limit || end {
			break
		}

		b, _ := json.Marshal(internal.NewEvent(
			struct{}{}, // unused
			EventSource,
			CtrlLeaderGetLatestRound,
		))

		outb, err := SendToLeader(ctx, fd.App, b, SendToLeaderExtra{RetryCount: 1})
		if err != nil {
			glog.Errorf("SendToLeader failed: %v", err)
			return err
		}

		var lpr lastPaxosRoundOutput
		json.Unmarshal(outb, &lpr)
		round := int(lpr.Round)
		if !lpr.Committed {
			round--
		}

		glog.Infof("[retry=%v] raw=%+v, latest=%v, committed=%v", retries, lpr, round, lpr.Committed)

		ins := make(map[int]struct{})
		vals, err := getValues(ctx, fd)
		if err != nil {
			glog.Errorf("getValues failed: %v", err)
			return err
		}

		startRound := 1
		for _, v := range vals {
			ins[int(v.Round)] = struct{}{}
			ss := strings.Split(v.Value, " ")
			if strings.HasPrefix(ss[0], "+") {
				if len(ss) > 2 {
					if strings.HasPrefix(ss[len(ss)-1], "_path:") {
						startRound = int(v.Round)
					}
				}
			}
		}

		missing := []int{}
		for i := startRound; i <= round; i++ {
			if _, ok := ins[i]; !ok {
				missing = append(missing, i)
			}
		}

		switch {
		case len(missing) > 0:
			glog.Infof("[retry=%v] missing=%v", retries, missing)
			bo := gaxv2.Backoff{} // 30s
			var endIn bool
			for i := 0; i < limit; i++ {
				if endIn {
					break
				}

				add := make(map[int]string)
				b, _ = json.Marshal(internal.NewEvent(
					LearnValuesInput{Rounds: missing},
					EventSource,
					CtrlBroadcastPaxosLearnValues,
				))

				var skipSelf bool
				if len(fd.App.FleetOp.Members()) > 1 {
					skipSelf = true
				}

				outs := fd.App.FleetOp.Broadcast(ctx, b, hedge.BroadcastArgs{SkipSelf: skipSelf})
				for _, out := range outs {
					var o []LearnValuesOutput
					json.Unmarshal(out.Reply, &o)
					for _, v := range o {
						add[int(v.Value.Round)] = v.Value.Value
					}
				}

				if len(missing) != len(add) {
					sleep := true
					if len(missing) == 1 && !lpr.Committed && len(add) == 0 {
						sleep = false // skip
					}

					if sleep {
						glog.Errorf("not matched, retry=%v/%v, missing=%v, add=%+v", retries, i, missing, add)
						time.Sleep(bo.Pause())
						continue
					}
				}

				endIn = true
				wmeta := []MetaT{}
				for k, v := range add {
					wmeta = append(wmeta, MetaT{
						Id:    fmt.Sprintf("%v/value", int64(*flags.Id)),
						Round: int64(k),
						Value: v,
					})
				}

				writeNodeMeta(ctx, &writeNoteMetaInput{
					FleetData: fd,
					Meta:      wmeta,
				})
			}
		default:
			// Update our replicated state machine.
			end = true
			fd.StateMachine.Reset()
			rounds := []int64{}
			for _, v := range vals {
				if v.Round >= int64(round) {
					if !lpr.Committed {
						continue
					}
				}

				fd.StateMachine.Apply(v.Value)
				atomic.StoreInt64(&fd.SetValueLastRound, v.Round)
				rounds = append(rounds, v.Round)
			}

			glog.Infof("[retry=%v] applied: rounds=%v, n=%v", retries, rounds, len(rounds))
		}
	}

	return nil
}

func broadcastRebuildRsmHeartbeat(ctx context.Context, fd *FleetData, paused chan struct{}) {
	ticker := time.NewTicker(time.Second)
	first := make(chan struct{}, 1)
	first <- struct{}{}
	defer ticker.Stop()

	var notifyPaused bool
	var active int32
	do := func() {
		atomic.StoreInt32(&active, 1)
		defer atomic.StoreInt32(&active, 0)
		b, _ := json.Marshal(internal.NewEvent(
			struct{}{},
			EventSource,
			CtrlBroadcastWipRebuildRsm,
		))

		var skipSelf bool
		if len(fd.App.FleetOp.Members()) > 1 {
			skipSelf = true
		}

		outs := fd.App.FleetOp.Broadcast(ctx, b, hedge.BroadcastArgs{SkipSelf: skipSelf})
		if len(outs) == 1 {
			paused <- struct{}{}
			notifyPaused = true
		}

		if !notifyPaused && len(outs) > 1 {
			for _, out := range outs {
				if out.Id != fd.App.FleetOp.HostPort() {
					paused <- struct{}{}
					notifyPaused = true
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-first:
		case <-ticker.C:
		}

		if atomic.LoadInt32(&active) == 1 {
			continue
		}

		go do()
	}
}

// MonitorRsmDrift monitors our local replicated log of missing indeces.
func MonitorRsmDrift(ctx context.Context, fd *FleetData) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	var startSnapshot int64
	var lastRound int64
	var active int32 // do() is running, wait next round
	do := func() {
		atomic.StoreInt32(&active, 1)
		defer atomic.StoreInt32(&active, 0)

		var lpr lastPaxosRoundOutput
		switch {
		case IsLeader(fd):
			var err error
			lpr, err = getLastPaxosRound(ctx, fd)
			if err != nil {
				glog.Errorf("getLastPaxosRound failed: %v", err)
				return
			}
		default:
			b, _ := json.Marshal(internal.NewEvent(
				struct{}{}, // unused
				EventSource,
				CtrlLeaderGetLatestRound,
			))

			outb, err := SendToLeader(ctx, fd.App, b, SendToLeaderExtra{RetryCount: 1})
			if err != nil {
				glog.Errorf("SendToLeader failed: %v", err)
				return
			}

			json.Unmarshal(outb, &lpr)
		}

		round := int(lpr.Round)
		if !lpr.Committed {
			round--
		}

		ins := make(map[int]struct{})
		vals, err := getValues(ctx, fd)
		if err != nil {
			glog.Errorf("getValues failed: %v", err)
			return
		}

		startRound := 1
		for _, v := range vals {
			ins[int(v.Round)] = struct{}{}
			ss := strings.Split(v.Value, " ")
			if strings.HasPrefix(ss[0], "+") {
				if len(ss) > 2 {
					if strings.HasPrefix(ss[len(ss)-1], "_path:") {
						startRound = int(v.Round)
					}
				}
			}
		}

		glog.Infof("fn:MonitorRsmDrift: latest=%+v, committed=%v, leader=%v, me=%v, len(set[1])=%v, len(set[2])=%v",
			round,
			lpr.Committed,
			atomic.LoadInt64(&fd.App.LeaderId),
			*flags.Id,
			len(fd.StateMachine.Members("1")),
			len(fd.StateMachine.Members("2")),
		)

		var term bool
		for i := startRound; i < round; i++ { // we are more interested in the in-betweens
			if _, ok := ins[i]; !ok {
				glog.Infof("[%v] _____missing [%v] in our rsm", fd.App.FleetOp.HostPort(), i)
				payload := fmt.Sprintf("[id%v/%v] missing [%v] in our rsm",
					*flags.Id,
					atomic.LoadInt64(&fd.App.LeaderId),
					i,
				)

				internal.TraceSlack(payload, "missing rsm")
				term = true
				break
			}
		}

		if term {
			// For now, if we have a missing round in our rsm, we terminate.
			// Allow the startup sequence to rebuild our local rsm.
			fd.App.TerminateCh <- struct{}{} // terminate self
		} else {
			atomic.StoreInt32(&fd.Online, 1) // tell API layer we are ready
		}

		if IsLeader(fd) { // let the leader initiate snapshotting
			lr := atomic.LoadInt64(&fd.SetValueLastRound)
			atomic.StoreInt64(&lastRound, lr)
			if atomic.LoadInt64(&lastRound) == lr {
				n := atomic.AddInt64(&startSnapshot, 1)
				if n%12 == 0 { // every minute of no activity
					b, _ := json.Marshal(internal.NewEvent(
						struct{}{},
						EventSource,
						CtrlBroadcastDoSnapshot,
					))

					fd.App.FleetOp.Broadcast(ctx, b)
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if atomic.LoadInt32(&active) == 1 {
			continue
		}

		go do()
	}
}
