package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/flags"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
	"github.com/google/uuid"
)

var (
	CmdTypeGetLock     = "CMDTYPE_GET_LOCK"
	CmdTypeExtendLock  = "CMDTYPE_EXTEND_LOCK"
	CmdTypeReleaseLock = "CMDTYPE_RELEASE_LOCK"
	CmdTypeAddToSet    = "CMDTYPE_ADDTOSET"
)

// Phase 1a:
type Prepare struct {
	PrepareId int64 `json:"prepareId"` // proposal
	NodeId    int64 `json:"nodeId"`    // tie-breaker
	Round     int64 `json:"round"`     // multipaxos round number
}

// Phase 1b:
type Promise struct {
	Error          error  `json:"error"`     // non-nil = NACK
	PrepareId      int64  `json:"prepareId"` // proposal
	NodeId         int64  `json:"nodeId"`    // tie-breaker
	Round          int64  `json:"round"`     // multipaxos round number
	LastPrepareId  int64  `json:"lastPrepareId"`
	LastAcceptedId int64  `json:"lastAcceptedId"`
	Value          string `json:"value"`
}

// Phase 2a:
type Accept struct {
	Round    int64  `json:"round"` // multipaxos round number
	AcceptId int64  `json:"acceptId"`
	NodeId   int64  `json:"nodeId"` // tie-breaker
	Value    string `json:"value"`
}

// Phase 2b:
type Accepted struct {
	Error    error `json:"error"` // non-nil = NACK
	Round    int64 `json:"round"` // multipaxos round number
	AcceptId int64 `json:"acceptId"`
	NodeId   int64 `json:"nodeId"` // tie-breaker
}

type sMetaT struct {
	Id        spanner.NullString
	Round     spanner.NullInt64
	Value     spanner.NullString
	Updated   spanner.NullTime
	Committed spanner.NullTime
}

type MetaT struct {
	Id        string    `json:"id"`
	Round     int64     `json:"round"`
	Value     string    `json:"value"`
	Updated   time.Time `json:"updated"`
	Committed time.Time `json:"committed"`
}

type lastPaxosRoundOutput struct {
	Round     int64  `json:"round"`
	Value     string `json:"value"`
	Committed bool   `json:"committed"`
}

// getLastPaxosRound returns the last/current paxos round, and an indication if
// the round is already committed (false = still ongoing).
func getLastPaxosRound(ctx context.Context, fd *FleetData) (lastPaxosRoundOutput, error) {
	var out lastPaxosRoundOutput
	var q strings.Builder
	fmt.Fprintf(&q, "select round, value, updated, committed from %s ", *flags.Meta)
	fmt.Fprintf(&q, "where id = 'chain' and ")
	fmt.Fprintf(&q, "((updated = committed) or (updated is not null and committed is null)) ")
	fmt.Fprintf(&q, "order by round desc limit 1")
	in := &internal.QuerySpannerSingleInput{
		Client: fd.App.Client,
		Query:  q.String(),
	}

	rows, err := internal.QuerySpannerSingle(ctx, in)
	if err != nil {
		return out, err
	}

	for _, row := range rows {
		var v sMetaT
		err = row.ToStruct(&v)
		if err != nil {
			return out, err
		}

		out.Round = v.Round.Int64
		out.Value = internal.SpannerString(v.Value)
		out.Committed = !v.Committed.IsNull()
		return out, nil
	}

	out.Committed = true // first entry
	return out, nil
}

// getValues gets all values for a specific node to apply to its internal rsm.
func getValues(ctx context.Context, fd *FleetData) ([]MetaT, error) {
	out := []MetaT{}
	var q strings.Builder
	fmt.Fprintf(&q, "select round, value from %s ", *flags.Meta)
	fmt.Fprintf(&q, "where id = '%v/value' order by round asc", *flags.Id)

	in := &internal.QuerySpannerSingleInput{
		Client: fd.App.Client,
		Query:  q.String(),
	}

	rows, err := internal.QuerySpannerSingle(ctx, in)
	if err != nil {
		return out, err
	}

	for _, row := range rows {
		var v sMetaT
		err = row.ToStruct(&v)
		if err != nil {
			return out, err
		}

		out = append(out, MetaT{
			Round: v.Round.Int64,
			Value: internal.SpannerString(v.Value),
		})
	}

	return out, nil
}

type writeMetaInput struct {
	FleetData *FleetData
	Meta      MetaT
}

func writeMeta(ctx context.Context, in writeMetaInput) error {
	_, err := in.FleetData.App.Client.Apply(ctx, []*spanner.Mutation{
		spanner.InsertOrUpdate(*flags.Meta,
			[]string{"id", "round", "value", "updated"},
			[]interface{}{in.Meta.Id, in.Meta.Round, in.Meta.Value, spanner.CommitTimestamp},
		),
	})

	return err
}

func commitMeta(ctx context.Context, in writeMetaInput) error {
	_, err := in.FleetData.App.Client.ReadWriteTransaction(ctx,
		func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			var q strings.Builder
			fmt.Fprintf(&q, "update %s ", *flags.Meta)
			fmt.Fprintf(&q, "set committed = (select updated from %s ", *flags.Meta)
			fmt.Fprintf(&q, "where id = @id and round = @round) ")
			fmt.Fprintf(&q, "where id = @id and round = @round")

			stmt := spanner.Statement{
				SQL:    q.String(),
				Params: map[string]interface{}{"id": in.Meta.Id, "round": in.Meta.Round},
			}

			_, err := txn.Update(ctx, stmt)
			return err
		},
	)

	return err
}

type writeNoteMetaInput struct {
	FleetData *FleetData
	Meta      []MetaT
}

func writeNodeMeta(ctx context.Context, in *writeNoteMetaInput) error {
	var n int
	cols := []string{"id", "round", "value", "updated"}
	m := make(map[int][]*spanner.Mutation)
	for _, v := range in.Meta {
		add := []interface{}{
			v.Id,
			v.Round,
			v.Value,
			spanner.CommitTimestamp,
		}

		mut := spanner.InsertOrUpdate(*flags.Meta, cols, add)
		idx := n / 5000 // mutation limit: ~(20000)/4cols
		if m[idx] == nil {
			m[idx] = []*spanner.Mutation{}
		}

		m[idx] = append(m[idx], mut)
		n++
	}

	// Actual spanner db writes.
	var sperrs []error
	func() {
		for _, recs := range m {
			_, err := in.FleetData.App.Client.Apply(ctx, recs)
			if err != nil {
				glog.Errorf("spanner.Apply failed: %v", err)
				sperrs = append(sperrs, err)
			}
		}
	}()

	if len(sperrs) > 0 {
		return fmt.Errorf("%v", sperrs)
	}

	return nil
}

type ReachConsensusInput struct {
	FleetData *FleetData `json:"fd,omitempty"`
	CmdType   string     `json:"cmdType,omitempty"`
	Key       string     `json:"key,omitempty"`
	Value     string     `json:"value,omitempty"`

	fwd bool `json:"-"` // so we know it's called via forwarding
}

type ReachConsensusOutput struct {
	Round int64  `json:"round,omitempty"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// ReachConsensus is our generic function to reach consensus on a value across a quorum of nodes
// using the multi-paxos algorithm variant. Normally called by the leader.
func ReachConsensus(ctx context.Context, in *ReachConsensusInput) (*ReachConsensusOutput, error) {
	if atomic.LoadInt64(&in.FleetData.BuildRsmOn) > 0 {
		return nil, ErrWipRebuild
	}

	in.FleetData.consensusMutex.Lock()
	defer in.FleetData.consensusMutex.Unlock()

	bctx := context.Background()
	out, err := getLastPaxosRound(ctx, in.FleetData)

	// This is our way of attempting to reset a stuck chain/round number.
	if !out.Committed {
		in.FleetData.App.Client.ReadWriteTransaction(bctx,
			func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				var q strings.Builder
				fmt.Fprintf(&q, "delete from %s ", *flags.Meta)
				fmt.Fprintf(&q, "where id = 'chain' and round = %v", out.Round)
				_, err := txn.Update(ctx, spanner.Statement{SQL: q.String()})
				return err
			},
		)

		return nil, fmt.Errorf("Operation pending. Please try again later.")
	}

	nextRound := out.Round + 1
	err = writeMeta(bctx, writeMetaInput{
		FleetData: in.FleetData,
		Meta: MetaT{
			Id:    "chain",
			Round: nextRound,
			Value: fmt.Sprintf("%v/%v", in.FleetData.App.FleetOp.HostPort(), *flags.Id),
		},
	})

	if err != nil {
		return nil, err
	}

	var commit bool
	defer func(pCommit *bool) {
		if !*pCommit {
			return
		}

		err := commitMeta(bctx, writeMetaInput{
			FleetData: in.FleetData,
			Meta: MetaT{
				Id:    "chain",
				Round: nextRound,
			},
		})

		if err != nil {
			glog.Errorf("commitMeta failed: %v", err)
		}
	}(&commit)

	var value string
	switch in.CmdType {
	case CmdTypeGetLock:
		token := uuid.NewString()
		now := internal.GetSpannerCurrentTime(in.FleetData.App.Client)
		value = fmt.Sprintf("/%v %v %v %v",
			in.Key, token, now.Format(time.RFC3339), in.Value) // fmt: </name token startTime durationInSec>
	case CmdTypeAddToSet:
		value = fmt.Sprintf("+%v %v", in.Key, in.Value) // fmt: <+key value>
	default:
		return nil, fmt.Errorf("Unsupported command [%v].", in.CmdType)
	}

	// Multi-Paxos: skip prepare, proceed directly to accept phase.
	ch := make(chan hedge.BroadcastOutput)
	b, _ := json.Marshal(internal.NewEvent(
		Accept{
			Round:    nextRound,
			AcceptId: 0,
			NodeId:   int64(*flags.Id),
			Value:    value,
		},
		EventSource,
		CtrlBroadcastPaxosPhase2Accept,
	))

	go in.FleetData.App.FleetOp.Broadcast(ctx, b, hedge.BroadcastArgs{Out: ch})

	chIn := make(chan *hedge.BroadcastOutput, *flags.NodeCount+1) // include exit msg
	majority := (*flags.NodeCount / 2) + 1
	done := make(chan int, 1)

	// NOTE: We only need the majority of the fleet's votes.
	go func() {
		var got int
		defer func(n *int) { done <- *n }(&got)

		for {
			m := <-chIn
			if m == nil {
				return
			}

			if m.Error != nil {
				continue // skip failures
			}

			var accept Accepted
			json.Unmarshal(m.Reply, &accept)
			if accept.Error == nil { // accepted
				got++
			}

			glog.Infof("got=%v, majority=%v, reply=%v", got, majority, string(m.Reply))
			if got >= majority { // quorum
				return
			}
		}
	}()

	// NOTE: Function could exit even if this goroutine hasn't finished yet.
	go func() {
		defer func() { chIn <- nil }() // for exit if no quorum
		for v := range ch {
			chIn <- &v
		}
	}()

	got := <-done
	if got < majority {
		return nil, fmt.Errorf("Quorum not reached.")
	}

	// We have agreed on a value. Broadcast to all.
	// NOTE: We assume that the caller for this is always the leader, direct or broadcast.
	// This write is to ensure that the leader will always have the latest agreed value.
	writeMeta(ctx, writeMetaInput{
		FleetData: in.FleetData,
		Meta: MetaT{
			Id:    fmt.Sprintf("%v/value", int64(*flags.Id)),
			Round: nextRound,
			Value: value,
		},
	})

	b, _ = json.Marshal(internal.NewEvent(
		Accept{ // reuse this struct
			Round: nextRound,
			Value: value,
		},
		EventSource,
		CtrlBroadcastPaxosSetValue,
	))

	in.FleetData.App.FleetOp.Broadcast(ctx, b)

	commit = true // commit our round number (see defer)
	return &ReachConsensusOutput{
		Round: nextRound,
		Key:   in.Key,
		Value: value,
	}, nil
}
