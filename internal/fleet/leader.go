package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/appdata"
	"github.com/alphauslabs/juno/internal/flags"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
	gaxv2 "github.com/googleapis/gax-go/v2"
)

var (
	CtrlLeaderPingPong       = "CTRL_PING_PONG"
	CtrlLeaderFwdConsensus   = "CTRL_LEADER_FWD_CONSENSUS"
	CtrlLeaderGetLatestRound = "CTRL_LEADER_GET_LATEST_ROUND"

	fnLeader = map[string]func(*FleetData, *cloudevents.Event) ([]byte, error){
		CtrlLeaderPingPong:       doLeaderPingPong,
		CtrlLeaderFwdConsensus:   doLeaderFwdConsensus,
		CtrlLeaderGetLatestRound: doLeaderGetLatestRound,
	}
)

func LeaderHandler(data interface{}, msg []byte) ([]byte, error) {
	fd := data.(*FleetData)
	var e cloudevents.Event
	err := json.Unmarshal(msg, &e)
	if err != nil {
		glog.Errorf("Unmarshal failed: %v", err)
		return nil, err
	}

	if _, ok := fnLeader[e.Type()]; !ok {
		return nil, fmt.Errorf("failed: unsupported type: %v", e.Type())
	}

	return fnLeader[e.Type()](fd, &e)
}

func doLeaderPingPong(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	switch {
	case string(e.Data()) != "PING":
		return nil, fmt.Errorf("invalid message")
	default:
		return []byte("PONG"), nil
	}
}

func doLeaderFwdConsensus(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	var data ReachConsensusInput
	err := json.Unmarshal(e.Data(), &data)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	data.FleetData = fd
	data.fwd = true
	out, err := ReachConsensus(ctx, &data)
	outb, _ := json.Marshal(out)
	return outb, err
}

func doLeaderGetLatestRound(fd *FleetData, e *cloudevents.Event) ([]byte, error) {
	ctx := context.Background()
	out, err := getLastPaxosRound(ctx, fd)
	outb, _ := json.Marshal(out)
	return outb, err
}

func EnsureLeaderActive(ctx context.Context, app *appdata.AppData) (bool, error) {
	msg := internal.NewEvent([]byte("PING"), "jupiter", CtrlLeaderPingPong)
	b, _ := json.Marshal(msg)
	r, err := SendToLeader(ctx, app, b)
	if err != nil {
		return false, err
	}

	switch {
	case string(r) == "PONG":
		return true, nil
	default:
		return false, nil
	}
}

type SendToLeaderExtra struct {
	RetryCount int
	BackOff    *gaxv2.Backoff // use if non-nil
}

func SendToLeader(ctx context.Context, app *appdata.AppData, m []byte, extra ...SendToLeaderExtra) ([]byte, error) {
	result := make(chan []byte, 1)
	done := make(chan error, 1)
	go func() {
		var err error
		var res []byte
		defer func(b *[]byte, e *error) {
			result <- *b
			done <- *e
		}(&res, &err)

		bo := gaxv2.Backoff{Max: time.Minute}
		for i := 0; i < 10; i++ {
			if !app.FleetOp.IsRunning() {
				time.Sleep(bo.Pause())
				continue
			}
		}

		bo = gaxv2.Backoff{Max: time.Second * 10}
		limit := 3
		if len(extra) > 0 {
			if extra[0].BackOff != nil {
				bo = *extra[0].BackOff
			}

			if extra[0].RetryCount > 0 {
				limit = extra[0].RetryCount
			}
		}

		for i := 0; i < limit; i++ {
			var r []byte
			r, err = app.FleetOp.Send(ctx, m)
			if err != nil {
				time.Sleep(bo.Pause())
				continue
			}

			res = r // to outside
			return
		}
	}()

	for {
		select {
		case e := <-done:
			return <-result, e
		case <-ctx.Done():
			return nil, context.Canceled
		}
	}
}

func LeaderLiveness(ctx context.Context, app *appdata.AppData) {
	ticker := time.NewTicker(time.Second * 3)
	var active int32

	do := func() {
		atomic.StoreInt32(&active, 1)
		defer atomic.StoreInt32(&active, 0)
		hl, _ := app.FleetOp.HasLock()
		if !hl {
			return // leader's job only
		}

		b, _ := json.Marshal(internal.NewEvent(
			hedge.KeyValue{Value: fmt.Sprintf("%v", *flags.Id)},
			EventSource,
			CtrlBroadcastLeaderLiveness,
		))

		outs := app.FleetOp.Broadcast(ctx, b)
		for i, out := range outs {
			if out.Error != nil {
				glog.Errorf("leader liveness: broadcast[%v] failed: %v", i, out.Error)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
		}

		if atomic.LoadInt32(&active) == 1 {
			continue
		}

		go do()
	}
}

func NoLeader(fd *FleetData) bool { return atomic.LoadInt64(&fd.App.LeaderId) < 1 }

func IsLeader(fd *FleetData) bool { return atomic.LoadInt64(&fd.App.LeaderId) == int64(*flags.Id) }
