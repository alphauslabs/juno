package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"cloud.google.com/go/spanner"
	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/flags"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
)

const (
	EventSource = "juno/internal"
)

var (
	CtrlBroadcastTest               = "CTRL_BROADCAST_TEST"
	CtrlBroadcastLeaderLiveness     = "CTRL_BROADCAST_LEADER_LIVENESS"
	CtrlBroadcastWipRebuildRsm      = "CTRL_BROADCAST_WIP_REBUILD_RSM"
	CtrlBroadcastPaxosPhase1Prepare = "CTRL_BROADCAST_PAXOS_PHASE1_PREPARE"
	CtrlBroadcastPaxosPhase2Accept  = "CTRL_BROADCAST_PAXOS_PHASE2_ACCEPT"
	CtrlBroadcastPaxosSetValue      = "CTRL_BROADCAST_PAXOS_SET_VALUE"
	CtrlBroadcastPaxosLearnValues   = "CTRL_BROADCAST_PAXOS_LEARN_VALUES"
	CtrlBroadcastDoSnapshot         = "CTRL_BROADCAST_DO_SNAPSHOT"

	fnBroadcast = map[string]func(*FleetData, *cloudevents.Event) ([]byte, error){
		CtrlBroadcastTest:               doBroadcastTest,
		CtrlBroadcastLeaderLiveness:     doBroadcastLeaderLiveness,
		CtrlBroadcastWipRebuildRsm:      doBroadcastWipRebuildRsm,
		CtrlBroadcastPaxosPhase1Prepare: doBroadcastPaxosPhase1Prepare,
		CtrlBroadcastPaxosPhase2Accept:  doBroadcastPaxosPhase2Accept,
		CtrlBroadcastPaxosSetValue:      doBroadcastPaxosSetValue,
		CtrlBroadcastPaxosLearnValues:   doBroadcastPaxosLearnValues,
		CtrlBroadcastDoSnapshot:         doBroadcastDoSnapshot,
	}
)

func BroadcastHandler(data interface{}, msg []byte) ([]byte, error) {
	fd := data.(*FleetData)
	var e cloudevents.Event
	err := json.Unmarshal(msg, &e)
	if err != nil {
		return nil, err
	}

	if _, ok := fnBroadcast[e.Type()]; !ok {
		return nil, fmt.Errorf("failed: unsupported type: %v", e.Type())
	}

	return fnBroadcast[e.Type()](fd, &e)
}

func doBroadcastTest(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	glog.Infof("[%v] test", fd.App.FleetOp.HostPort())
	return nil, nil
}

func doBroadcastLeaderLiveness(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	var data hedge.KeyValue
	err := json.Unmarshal(e.Data(), &data)
	if err == nil && data.Value != "" {
		f, _ := strconv.ParseInt(data.Value, 10, 64)
		atomic.StoreInt64(&fd.App.LeaderId, f)
	}

	if fd.App.LeaderActive != nil {
		fd.App.LeaderActive.On()
	}

	return nil, nil
}

func doBroadcastWipRebuildRsm(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	atomic.StoreInt64(&fd.BuildRsmOn, 1)
	fd.BuildRsmWip.On()
	return nil, nil
}

func doBroadcastPaxosPhase1Prepare(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	return nil, nil
}

func doBroadcastPaxosPhase2Accept(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	var data Accept
	json.Unmarshal(e.Data(), &data)
	out := Accepted{
		Round:    data.Round,
		AcceptId: data.AcceptId,
		NodeId:   int64(*flags.Id),
	}

	switch {
	case data.AcceptId == 0: // leader, always accept
		err := writeNodeMeta(context.Background(), &writeNoteMetaInput{
			FleetData: fd,
			Meta: []MetaT{
				{
					Id:    fmt.Sprintf("%v/lastPromisedId", int64(*flags.Id)),
					Round: data.Round,
					Value: "0",
				},
				{
					Id:    fmt.Sprintf("%v/lastAcceptedId", int64(*flags.Id)),
					Round: data.Round,
					Value: "0",
				},
				{
					Id:    fmt.Sprintf("%v/lastValue", int64(*flags.Id)),
					Round: data.Round,
					Value: data.Value,
				},
			},
		})

		if err != nil {
			glog.Errorf("writeNodeMeta failed: %v", err)
			return nil, err
		}
	default:
		glog.Infof("accept: not leader, NACK")
		out.Error = fmt.Errorf("NACK: not leader (tmp).")
	}

	outb, _ := json.Marshal(out)
	return outb, nil
}

func doBroadcastPaxosSetValue(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	if atomic.LoadInt32(&fd.SetValueReady) == 0 {
		return nil, fmt.Errorf("node [%v] not ready", *flags.Id)
	}

	var data Accept // reuse this struct for the broadcast
	json.Unmarshal(e.Data(), &data)
	err := writeMeta(context.Background(), writeMetaInput{
		FleetData: fd,
		Meta: MetaT{
			Id:    fmt.Sprintf("%v/value", int64(*flags.Id)),
			Round: data.Round,
			Value: data.Value,
		},
	})

	if err != nil {
		return nil, err
	}

	diff := data.Round - atomic.LoadInt64(&fd.SetValueLastRound)
	switch {
	case diff > 1: // for later
		// For now, if we have a missing round in our rsm, we terminate.
		// Allow the startup sequence to rebuild our local rsm.
		glog.Infof("rsm behind, quit: round=%v, value=%v, diff=%v", data.Round, data.Value, diff)
		fd.App.TerminateCh <- struct{}{} // terminate self
	case diff == 1: // expected round
		fd.StateMachine.Apply(data.Value)
		atomic.StoreInt64(&fd.SetValueLastRound, data.Round)
		glog.Infof("applyNow[this]: round=%v, value=%v", data.Round, data.Value)

		fd.SetValueMtx.Lock()
		fd.SetValueHistory[int(data.Round)] = &SetValueHistoryT{
			Value:   data.Value,
			Applied: true,
		}

		n := int(data.Round)
		for {
			n++
			if _, ok := fd.SetValueHistory[n]; !ok {
				break
			}

			if !fd.SetValueHistory[n].Applied {
				fd.StateMachine.Apply(fd.SetValueHistory[n].Value)
				fd.SetValueHistory[n].Applied = true
				atomic.StoreInt64(&fd.SetValueLastRound, int64(n))
				glog.Infof("__applyNow[hist]: round=%v, value=%v", n, fd.SetValueHistory[n].Value)
			}
		}

		fd.SetValueMtx.Unlock()
	}

	return nil, nil
}

type LearnValuesInput struct {
	Rounds []int `json:"rounds"`
}

type LearnValuesOutput struct {
	Value Accept `json:"value"` // reuse
}

func doBroadcastPaxosLearnValues(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	var data LearnValuesInput
	json.Unmarshal(e.Data(), &data)
	rounds := []string{}
	for _, i := range data.Rounds {
		rounds = append(rounds, fmt.Sprintf("%v", i))
	}

	out := []LearnValuesOutput{}
	var q strings.Builder
	fmt.Fprintf(&q, "select round, value from %s ", *flags.Meta)
	fmt.Fprintf(&q, "where id = '%v/value' and round in (%s)", *flags.Id, strings.Join(rounds, ","))

	in := &internal.QuerySpannerSingleInput{
		Client: fd.App.Client,
		Query:  q.String(),
	}

	ctx := context.Background()
	rows, err := internal.QuerySpannerSingle(ctx, in)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		var v sMetaT
		err = row.ToStruct(&v)
		if err != nil {
			return nil, err
		}

		out = append(out, LearnValuesOutput{
			Value: Accept{
				Round: v.Round.Int64,
				Value: internal.SpannerString(v.Value),
			},
		})
	}

	outb, _ := json.Marshal(out)
	return outb, nil
}

func doBroadcastDoSnapshot(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	ctx := context.Background()
	outs, err := getValues(ctx, fd)
	if err != nil {
		glog.Errorf("getValues failed, wait next round: %v", err)
		return nil, nil
	}

	limit := len(outs) - 2
	rsm := NewRsm()
	var snapshotRound int64
	var snapshotValue string
	for i, out := range outs {
		if i >= limit {
			break
		}

		snapshotRound = out.Round
		snapshotValue = out.Value
		rsm.Apply(out.Value)
	}

	if snapshotRound < 1 {
		return nil, nil
	}

	object := fmt.Sprintf("juno/%v-%v.json", *flags.Id, snapshotRound)
	copy := rsm.Clone()
	b, _ := json.Marshal(copy)
	err = internal.PutSnapshot(object, b)
	if err != nil {
		glog.Errorf("PutSnapshot failed: %v", err)
		return nil, nil
	}

	glog.Infof("snapshot at [%v] for node%v, len=%v", snapshotRound, *flags.Id, len(b))

	_, err = fd.App.Client.ReadWriteTransaction(ctx,
		func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			var q strings.Builder
			fmt.Fprintf(&q, "update %s ", *flags.Meta)
			fmt.Fprintf(&q, "set value = @val ")
			fmt.Fprintf(&q, "where id = @id and round = @round")
			stmt := spanner.Statement{
				SQL: q.String(),
				Params: map[string]interface{}{
					"id":    fmt.Sprintf("%v/value", *flags.Id),
					"round": snapshotRound,
					"val":   fmt.Sprintf("%v _path:%v", snapshotValue, object),
				},
			}

			_, err := txn.Update(ctx, stmt)
			return err
		},
	)

	if err != nil {
		glog.Errorf("Update (_path:%v) failed: %v", object, err)
		return nil, nil
	}

	_, err = fd.App.Client.ReadWriteTransaction(ctx,
		func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			var q strings.Builder
			fmt.Fprintf(&q, "delete from %s ", *flags.Meta)
			fmt.Fprintf(&q, "where id = '%v/value' and round < %v", *flags.Id, snapshotRound)
			_, err := txn.Update(ctx, spanner.Statement{SQL: q.String()})
			return err
		},
	)

	if err != nil {
		glog.Errorf("Update (delete < %v) failed: %v", snapshotRound, err)
	}

	return nil, nil
}
