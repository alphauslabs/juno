package fleet

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/alphauslabs/juno/internal"
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

// Reset deletes the map under the input key.
func (s *rsmT) Reset(key string) {
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

// NewRsm returns an instance of our replicated state machine set.
func NewRsm() *rsmT {
	return &rsmT{set: map[string]map[string]struct{}{}}
}

// BuildSet builds the state machine to it's current state from our replicated log.
func BuildSet(ctx context.Context, fd *FleetData) error {
	defer func(begin time.Time) { glog.Infof("fn BuildSet took %v", time.Since(begin)) }(time.Now())

	b, _ := json.Marshal(internal.NewEvent(
		struct{}{}, // unused
		EventSource,
		CtrlLeaderGetLatestRound,
	))

	outb, err := SendToLeader(ctx, fd.App, b, SendToLeaderExtra{RetryCount: 1})
	if err != nil {
		glog.Errorf("BuildSet failed: %v", err)
		return err
	}

	var out lastPaxosRoundOutput
	err = json.Unmarshal(outb, &out)
	if err != nil {
		glog.Errorf("Unmarshal failed: %v", err)
		return err
	}

	glog.Infof("latest=%+v", out)

	ins := make(map[int]struct{})
	vals, err := getValues(ctx, fd)
	if err != nil {
		glog.Errorf("BuildSet failed: %v", err)
		return err
	}

	for _, v := range vals {
		ins[int(v.Round)] = struct{}{}
	}

	missing := []int{}
	for i := 1; i <= int(out.Round); i++ {
		if _, ok := ins[i]; !ok {
			missing = append(missing, i)
		}
	}

	if len(missing) > 0 {
		glog.Infof("missing rounds: %v", missing)
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
				glog.Errorf("something is wrong with lens, %v, %v, %v, %+v", len(missing), len(add), missing, add)
				time.Sleep(bo.Pause())
			} else {
				break
			}
		}
	}

	return nil
}
