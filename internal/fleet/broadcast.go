package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

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

	fnBroadcast = map[string]func(*FleetData, *cloudevents.Event) ([]byte, error){
		CtrlBroadcastTest:               doBroadcastTest,
		CtrlBroadcastLeaderLiveness:     doBroadcastLeaderLiveness,
		CtrlBroadcastWipRebuildRsm:      doBroadcastWipRebuildRsm,
		CtrlBroadcastPaxosPhase1Prepare: doBroadcastPaxosPhase1Prepare,
		CtrlBroadcastPaxosPhase2Accept:  doBroadcastPaxosPhase2Accept,
		CtrlBroadcastPaxosSetValue:      doBroadcastPaxosSetValue,
		CtrlBroadcastPaxosLearnValues:   doBroadcastPaxosLearnValues,
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
			Meta: []metaT{
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
	var data Accept // reuse this struct for the broadcast
	json.Unmarshal(e.Data(), &data)
	err := writeMeta(context.Background(), writeMetaInput{
		FleetData: fd,
		Meta: metaT{
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
		glog.Infof("__saveForLater: round=%v, value=%v", data.Round, data.Value)
		fd.muSetValue.Lock()
		fd.SetValueTempPipe[int(data.Round)] = data.Value
		fd.muSetValue.Unlock()
	case diff == 1: // expected round
		fd.StateMachine.Apply(data.Value)
		fd.muSetValue.Lock()
		copy := fd.SetValueTempPipe
		fd.SetValueTempPipe = map[int]string{} // clear
		fd.muSetValue.Unlock()

		lastRound := data.Round
		rounds := []int{}
		if len(copy) > 0 {
			for k := range copy {
				rounds = append(rounds, k)
			}

			sort.Ints(rounds)
			lastRound = int64(rounds[len(rounds)-1])
			for _, n := range rounds {
				fd.StateMachine.Apply(copy[n])
			}
		}

		glog.Infof("__applyNow: round=%v, value=%v, also=%v", data.Round, data.Value, rounds)
		atomic.StoreInt64(&fd.SetValueLastRound, lastRound)
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
