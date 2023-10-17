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

type FleetData struct {
	App *appdata.AppData // global appdata

	StateMachine *rsmT // our replicated state machine

	BuildRsmWip *timedoff.TimedOff // pause ops when active
	BuildRsmOn  int64              // non-zero means someone is rebuilding their RSM

	consensusMutex sync.Mutex // makes calling ReachConsensus synchronous
}
