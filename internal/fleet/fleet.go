package fleet

import (
	"fmt"
	"sync"

	"github.com/alphauslabs/juno/internal/appdata"
)

var (
	ErrClusterOffline = fmt.Errorf("juno: Cluster not running.")
	ErrNoLeader       = fmt.Errorf("juno: Leader unavailable. Please try again later.")
)

type FleetData struct {
	App *appdata.AppData // global appdata

	// Our replicated set map. Mainly used for task completion.
	// Think of Redis sets (SADD, SMEMBERS, etc.) but replicated.
	Set *rsmT

	consensusMutex sync.Mutex // makes calling ReachConsensus synchronous
}
