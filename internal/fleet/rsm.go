package fleet

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/alphauslabs/juno/internal"
	"github.com/golang/glog"
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
		RoundInfo{RoundNum: 10},
		EventSource,
		CtrlLeaderGetRoundInfo,
	))

	outb, err := SendToLeader(ctx, fd.App, b)
	if err != nil {
		glog.Errorf("BuildSet failed: %v", err)
		return err
	}

	var ri RoundInfo
	json.Unmarshal(outb, &ri)
	refSeq := make(map[int]struct{})
	for i := 1; i <= int(ri.RoundNum); i++ {
		refSeq[i] = struct{}{}
	}

	vals, err := getValues(ctx, fd)
	if err != nil {
		glog.Errorf("BuildSet failed: %v", err)
		return err
	}

	for _, v := range vals {
		if _, ok := refSeq[int(v.RoundNum)]; !ok {
			glog.Infof("todo: missing round %v", v.RoundNum)
		}
	}

	return nil
}
