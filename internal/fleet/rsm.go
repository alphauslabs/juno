package fleet

import (
	"context"
	"encoding/json"
	"fmt"
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

type rsmT struct {
	mu  sync.RWMutex
	set map[string]map[string]struct{}
}

// Apply applies cmd to its internal state machine.
// The cmd format is <+key value>. The return value
// is the total number of items under the input key.
func (s *rsmT) Apply(cmd string) int {
	if !strings.HasPrefix(cmd, "+") {
		return -1 // error
	}

	ss := strings.Split(cmd[1:], " ")
	if len(ss) != 2 {
		return -1 //error
	}

	s.mu.Lock()
	if _, ok := s.set[ss[0]]; !ok {
		s.set[ss[0]] = make(map[string]struct{})
	}

	s.set[ss[0]][ss[1]] = struct{}{}
	n := len(s.set[ss[0]])
	s.mu.Unlock()
	return n
}

// Members return the current members of the input key.
func (s *rsmT) Members(key string) []string {
	m := []string{}
	s.mu.Lock()
	for k := range s.set[key] {
		m = append(m, k)
	}

	s.mu.Unlock()
	return m
}

// Delete deletes the map under the input key.
func (s *rsmT) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.set, key)
}

// Clone returns a copy of the internal set.
func (s *rsmT) Clone() map[string]map[string]struct{} {
	copy := make(map[string]map[string]struct{})
	s.mu.Lock()
	for k, v := range s.set {
		copy[k] = v
	}

	s.mu.Unlock()
	return copy
}

// Reset resets the underlying map.
func (s *rsmT) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set = map[string]map[string]struct{}{}
}

// NewRsm returns an instance of our replicated state machine set.
func NewRsm() *rsmT {
	return &rsmT{set: map[string]map[string]struct{}{}}
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

	retries := -1
	var end bool
	for {
		retries++
		if retries >= 10 || end {
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

		glog.Infof("[retry=%v] latest=%+v", retries, lpr)
		round := int(lpr.Round)
		if !lpr.Committed {
			round--
		}

		ins := make(map[int]struct{})
		vals, err := getValues(ctx, fd)
		if err != nil {
			glog.Errorf("getValues failed: %v", err)
			return err
		}

		for _, v := range vals {
			ins[int(v.Round)] = struct{}{}
		}

		missing := []int{}
		for i := 1; i <= round; i++ {
			if _, ok := ins[i]; !ok {
				missing = append(missing, i)
			}
		}

		switch {
		case len(missing) > 0:
			glog.Infof("[retry=%v] missing=%v", retries, missing)
			bo := gaxv2.Backoff{} // 30s
			var endIn bool
			for i := 0; i < 10; i++ {
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
				wmeta := []metaT{}
				for k, v := range add {
					wmeta = append(wmeta, metaT{
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
			if retries > 0 {
				fd.StateMachine.Reset()
				glog.Infof("[retry=%v] reset statemachine", retries)
			}

			end = true
			rounds := []int64{}
			for _, v := range vals {
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
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	var active int32
	do := func() {
		atomic.StoreInt32(&active, 1)
		defer atomic.StoreInt32(&active, 0)

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

		var lpr lastPaxosRoundOutput
		err = json.Unmarshal(outb, &lpr)
		if err != nil {
			glog.Errorf("Unmarshal failed: %v", err)
			return
		}

		glog.Infof("fn:MonitorRsmDrift: latest=%+v, leader=%v, me=%v",
			lpr,
			atomic.LoadInt64(&fd.App.LeaderId),
			*flags.Id,
		)

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

		for _, v := range vals {
			ins[int(v.Round)] = struct{}{}
		}

		for i := 1; i < round; i++ { // we are more interested in the in-betweens
			if _, ok := ins[i]; !ok {
				glog.Infof("[%v] _____missing [%v] in our rsm", fd.App.FleetOp.HostPort(), i)
				payload := fmt.Sprintf("[id%v/%v] missing [%v] in our rsm",
					*flags.Id,
					atomic.LoadInt64(&fd.App.LeaderId),
					i,
				)

				internal.TraceSlack(payload, "missing rsm")
				break
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
