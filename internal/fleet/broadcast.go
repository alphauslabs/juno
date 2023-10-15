package fleet

import (
	"encoding/json"
	"fmt"
	"unsafe"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
)

const (
	EventSource = "juno/internal"
)

var (
	ErrClusterOffline = fmt.Errorf("failed: cluster not running")

	CtrlBroadcastTest           = "CTRL_BROADCAST_TEST"
	CtrlBroadcastLeaderLiveness = "CTRL_BROADCAST_LEADER_LIVENESS"

	fnBroadcast = map[string]func(*FleetData, *cloudevents.Event) ([]byte, error){
		CtrlBroadcastTest:           doBroadcastTest,
		CtrlBroadcastLeaderLiveness: doBroadcastLeaderLiveness,
	}

	stringToBytes = func(s string) []byte {
		return *(*[]byte)(unsafe.Pointer(
			&struct {
				string
				int
			}{s, len(s)},
		))
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
		fd.App.Lock()
		fd.App.LeaderId = data.Value
		fd.App.Unlock()
	}

	fd.App.LeaderActive.On()
	return nil, nil
}
