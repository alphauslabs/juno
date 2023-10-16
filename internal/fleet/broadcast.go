package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/alphauslabs/juno/internal/flags"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
)

const (
	EventSource = "juno/internal"
)

var (
	ErrClusterOffline = fmt.Errorf("failed: cluster not running")

	CtrlBroadcastTest               = "CTRL_BROADCAST_TEST"
	CtrlBroadcastLeaderLiveness     = "CTRL_BROADCAST_LEADER_LIVENESS"
	CtrlBroadcastPaxosPhase1Prepare = "CTRL_BROADCAST_PAXOS_PHASE1_PREPARE"
	CtrlBroadcastPaxosPhase2Accept  = "CTRL_BROADCAST_PAXOS_PHASE2_ACCEPT"

	fnBroadcast = map[string]func(*FleetData, *cloudevents.Event) ([]byte, error){
		CtrlBroadcastTest:               doBroadcastTest,
		CtrlBroadcastLeaderLiveness:     doBroadcastLeaderLiveness,
		CtrlBroadcastPaxosPhase1Prepare: doBroadcastPaxosPhase1Prepare,
		CtrlBroadcastPaxosPhase2Accept:  doBroadcastPaxosPhase2Accept,
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

func doBroadcastPaxosPhase1Prepare(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	return nil, nil
}

func doBroadcastPaxosPhase2Accept(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	var data Accept
	err := json.Unmarshal(e.Data(), &data)
	if err != nil {
		return nil, err
	}

	out := Accepted{
		RoundNum: data.RoundNum,
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
					Round: data.RoundNum,
					Value: "0",
				},
				{
					Id:    fmt.Sprintf("%v/lastAcceptedId", int64(*flags.Id)),
					Round: data.RoundNum,
					Value: "0",
				},
				{
					Id:    fmt.Sprintf("%v/value", int64(*flags.Id)),
					Round: data.RoundNum,
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
