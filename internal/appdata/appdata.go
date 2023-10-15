package appdata

import (
	"cloud.google.com/go/spanner"
	"github.com/flowerinthenight/hedge"
	"github.com/flowerinthenight/timedoff"
)

type AppData struct {
	Client *spanner.Client

	// Our fleet orchestrator.
	FleetOp *hedge.Op

	// Our resettable timer telling us if we have a leader.
	LeaderActive *timedoff.TimedOff

	// The current leader's id. Zero means no leader.
	LeaderId int64
}
