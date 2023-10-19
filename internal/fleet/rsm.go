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
func BuildRsm(ctx context.Context, fd *FleetData) error {
	defer func(begin time.Time) { glog.Infof("fn:BuildRsm took %v", time.Since(begin)) }(time.Now())

	paused := make(chan struct{}, 1)
	nctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go BroadcastRebuildRsmWip(nctx, fd, paused)

	<-paused // wait for the signal that others have acknowledged pause

	retries := -1
	for {
		retries++
		if retries >= 3 {
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
		err = json.Unmarshal(outb, &lpr)
		if err != nil {
			glog.Errorf("Unmarshal failed: %v", err)
			return err
		}

		glog.Infof("latest=%+v", lpr)
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

		if len(missing) > 0 {
			glog.Infof("--> missing=%v", missing)
			bo := gaxv2.Backoff{} // 30s
			for i := 0; i < 10; i++ {
				add := make(map[int]string)
				b, _ = json.Marshal(internal.NewEvent(
					LearnValuesInput{Rounds: missing},
					EventSource,
					CtrlBroadcastPaxosLearnValues,
				))

				outs := fd.App.FleetOp.Broadcast(ctx, b)
				for _, out := range outs {
					var o []LearnValuesOutput
					err = json.Unmarshal(out.Reply, &o)
					if err != nil {
						glog.Errorf("Unmarshal failed for %v: %v", out.Id, err)
						continue
					}

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
						glog.Errorf("not matched, retry: missing=%v, add=%+v", missing, add)
						time.Sleep(bo.Pause())
						continue
					}
				}

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

				break // don't forget
			}
		} else {
			// Update our replicated state machine.
			if retries > 0 {
				fd.StateMachine.Reset()
			}

			for _, v := range vals {
				fd.StateMachine.Apply(v.Value)
			}

			break // main
		}
	}

	return nil
}

func BroadcastRebuildRsmWip(ctx context.Context, fd *FleetData, paused chan struct{}) {
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

		outs := fd.App.FleetOp.Broadcast(ctx, b)
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

		glog.Infof("[%v] MonitorRsmDrift: latest=%+v", fd.App.FleetOp.HostPort(), lpr)
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
				glog.Infof("[%v] _____missing [%v] in our logs, todo: restart?", fd.App.FleetOp.HostPort(), i)
				payload := fmt.Sprintf("[%v] missing [%v] in our rsm", fd.App.FleetOp.HostPort(), i)
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
