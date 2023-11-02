package fleet

import (
	"fmt"
	"sync"

	"github.com/alphauslabs/juno/internal/appdata"
	"github.com/flowerinthenight/timedoff"
)

var (
	ErrClusterOffline = fmt.Errorf("juno: Cluster not running.")
	ErrNoLeader       = fmt.Errorf("juno: Leader unavailable. Please try again later.")
	ErrWipRebuild     = fmt.Errorf("juno: State machine rebuild in progress. Please try again later.")
)

type SetValueHistoryT struct {
	Value   string
	Applied bool
}

type FleetData struct {
	App *appdata.AppData // global appdata

	StateMachine *rsmT // our replicated state machine

	BuildRsmWip *timedoff.TimedOff // pause ops when active
	BuildRsmOn  int64              // non-zero means someone is rebuilding their RSM

	Online int32 // non-zero mean we are ready to serve

	SetValueMtx       sync.RWMutex
	SetValueHistory   map[int]*SetValueHistoryT // track incoming rounds since online, not complete
	SetValueLastRound int64
	SetValueReady     int32

	consensusMutex sync.Mutex // makes calling ReachConsensus synchronous; leader only
}
