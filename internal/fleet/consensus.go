package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/flags"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
)

var (
	CmdTypeAddToSet = "CMDTYPE_ADDTOSET"
)

// Phase 1a:
type Prepare struct {
	PrepareId int64 `json:"prepareId"` // proposal
	NodeId    int64 `json:"nodeId"`    // tie-breaker
	RoundNum  int64 `json:"roundNum"`  // multipaxos round number
}

// Phase 1b:
type Promise struct {
	Error          error  `json:"error"`     // non-nil = NACK
	PrepareId      int64  `json:"prepareId"` // proposal
	NodeId         int64  `json:"nodeId"`    // tie-breaker
	RoundNum       int64  `json:"roundNum"`  // multipaxos round number
	LastPrepareId  int64  `json:"lastPrepareId"`
	LastAcceptedId int64  `json:"lastAcceptedId"`
	Value          string `json:"value"`
}

// Phase 2a:
type Accept struct {
	RoundNum int64  `json:"roundNum"` // multipaxos round number
	AcceptId int64  `json:"acceptId"`
	NodeId   int64  `json:"nodeId"` // tie-breaker
	Value    string `json:"value"`
}

// Phase 2b:
type Accepted struct {
	Error    error `json:"error"`    // non-nil = NACK
	RoundNum int64 `json:"roundNum"` // multipaxos round number
	AcceptId int64 `json:"acceptId"`
	NodeId   int64 `json:"nodeId"` // tie-breaker
}

type roundT struct {
	Round     spanner.NullInt64
	Updated   spanner.NullTime
	Committed spanner.NullTime
}

// getLastPaxosRound returns the last/current paxos round, and an indication if
// the round is already committed (false = still ongoing).
func getLastPaxosRound(ctx context.Context, fd *FleetData) (int64, bool, error) {
	var q strings.Builder
	fmt.Fprintf(&q, "select round, updated, committed from %s ", *flags.Meta)
	fmt.Fprintf(&q, "where id = 'chain' and ")
	fmt.Fprintf(&q, "((updated = committed) or (updated is not null and committed is null)) ")
	fmt.Fprintf(&q, "order by round desc limit 1")
	in := &internal.QuerySpannerSingleInput{
		Client: fd.App.Client,
		Query:  q.String(),
	}

	rows, err := internal.QuerySpannerSingle(ctx, in)
	if err != nil {
		return 0, true, err
	}

	for _, row := range rows {
		var v roundT
		err = row.ToStruct(&v)
		if err != nil {
			return 0, true, err
		}

		glog.Infof("meta: round=%v, updated=%v, committed=%v",
			v.Round.Int64, v.Updated.Time.Format(time.RFC3339), v.Committed.Time.Format(time.RFC3339))

		return v.Round.Int64, !v.Committed.IsNull(), nil
	}

	return 0, true, nil
}

type sMetaT struct {
	Id        spanner.NullString
	Round     spanner.NullInt64
	Value     spanner.NullString
	Updated   spanner.NullTime
	Committed spanner.NullTime
}

type RoundInfo struct {
	RoundNum int64  `json:"roundNum"` // multipaxos round number
	Value    string `json:"value"`    // agreed value for this round
}

// getRoundInfo gets the value of a specific multi-paxos round. Usually called by the leader.
func getRoundInfo(ctx context.Context, fd *FleetData, ri RoundInfo) (RoundInfo, error) {
	defer func(begin time.Time) { glog.Infof("getRoundInfo took %v", time.Since(begin)) }(time.Now())

	var out RoundInfo
	var q strings.Builder
	switch {
	case ri.RoundNum < 0: // get latest
		fmt.Fprintf(&q, "select round, value from %s ", *flags.Meta)
		fmt.Fprintf(&q, "where id = '%v/value' ", *flags.Id)
		fmt.Fprintf(&q, "and round = (select round from %s ", *flags.Meta)
		fmt.Fprintf(&q, "where id = 'chain' and updated is not null ")
		fmt.Fprintf(&q, "and committed is not null order by round desc limit 1)")
	default: // specific
		fmt.Fprintf(&q, "select round, value from %s ", *flags.Meta)
		fmt.Fprintf(&q, "where id = '%v/value' ", *flags.Id)
		fmt.Fprintf(&q, "and round = %v", ri.RoundNum)
	}

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

		out.RoundNum = v.Round.Int64
		out.Value = internal.SpannerString(v.Value)
		return out, nil
	}

	return out, fmt.Errorf("Round [%v] not found.", ri.RoundNum)
}

// getValues gets all values for a specific node to apply to its internal rsm.
func getValues(ctx context.Context, fd *FleetData) ([]RoundInfo, error) {
	defer func(begin time.Time) { glog.Infof("getValues took %v", time.Since(begin)) }(time.Now())

	out := []RoundInfo{}
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

		out = append(out, RoundInfo{
			RoundNum: v.Round.Int64,
			Value:    internal.SpannerString(v.Value),
		})
	}

	return out, nil
}

type metaT struct {
	Id    string
	Round int64
	Value string
}

type writeMetaInput struct {
	FleetData *FleetData
	Meta      metaT
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
	Meta      []metaT
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
		var n int
		defer func(begin time.Time, n *int) {
			glog.Infof("[spanner] %v written to %v, took %v", *n, *flags.Meta, time.Since(begin))
		}(time.Now(), &n)

		for _, recs := range m {
			n += len(recs)
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
}

type ReachConsensusOutput struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
	Count int    `json:"count,omitempty"`
}

// ReachConsensus is our generic function to reach consensus on a value across a quorum of nodes
// using the multi-paxos algorithm variant.
func ReachConsensus(ctx context.Context, in *ReachConsensusInput) (*ReachConsensusOutput, error) {
	round, committed, err := getLastPaxosRound(ctx, in.FleetData)
	glog.Infof("round=%v, committed=%v, err=%v", round, committed, err)

	if !committed {
		return nil, fmt.Errorf("Operation pending. Please try again later.")
	}

	err = writeMeta(ctx, writeMetaInput{
		FleetData: in.FleetData,
		Meta: metaT{
			Id:    "chain",
			Round: round + 1,
			Value: fmt.Sprintf("%v/%v", in.FleetData.App.FleetOp.HostPort(), *flags.Id),
		},
	})

	if err != nil {
		return nil, err
	}

	defer func() {
		err := commitMeta(ctx, writeMetaInput{
			FleetData: in.FleetData,
			Meta: metaT{
				Id:    "chain",
				Round: round + 1,
			},
		})

		if err != nil {
			glog.Errorf("commitMeta failed: %v", err)
		}
	}()

	var value string
	switch in.CmdType {
	case CmdTypeAddToSet:
		value = fmt.Sprintf("+%v %v", in.Key, in.Value) // addtoset fmt: <+key value>
	default:
		return nil, fmt.Errorf("Unsupported command [%v].", in.CmdType)
	}

	// Multi-Paxos: skip prepare, proceed directly to accept phase.
	ch := make(chan hedge.BroadcastOutput)
	b, _ := json.Marshal(internal.NewEvent(
		Accept{
			RoundNum: round + 1,
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

			glog.Infof("reply=%v", string(m.Reply))
			var accept Accepted
			err := json.Unmarshal(m.Reply, &accept)
			if err != nil {
				glog.Errorf("Unmarshal failed: %v", err)
				continue
			}

			if accept.Error == nil { // accepted
				got++
			}

			glog.Infof("got=%v, majority=%v", got, majority)
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

	return &ReachConsensusOutput{Key: in.Key, Value: in.Value}, nil
}
