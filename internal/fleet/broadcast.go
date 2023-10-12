package fleet

import (
	"encoding/json"
	"fmt"
	"unsafe"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/golang/glog"
)

const (
	EventSource = "juno/internal"
)

var (
	ErrClusterOffline = fmt.Errorf("failed: cluster not running")

	CtrlBroadcastLeaderLiveness = "CTRL_BROADCAST_LEADER_LIVENESS"

	fnBroadcast = map[string]func(*FleetData, *cloudevents.Event) ([]byte, error){
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
	cd := data.(*FleetData)
	var e cloudevents.Event
	err := json.Unmarshal(msg, &e)
	if err != nil {
		glog.Errorf("Unmarshal failed: %v", err)
		return nil, err
	}

	if _, ok := fnBroadcast[e.Type()]; !ok {
		return nil, fmt.Errorf("failed: unsupported type: %v", e.Type())
	}

	return fnBroadcast[e.Type()](cd, &e)
}

func doBroadcastLeaderLiveness(cd *FleetData, e *cloudevents.Event) ([]byte, error) {
	cd.App.LeaderActive.On()
	return nil, nil
}
