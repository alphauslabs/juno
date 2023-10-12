package appdata

import (
	"sync"

	"cloud.google.com/go/spanner"
	"github.com/flowerinthenight/hedge"
	"github.com/flowerinthenight/timedoff"
)

type AppData struct {
	sync.Mutex
	Client       *spanner.Client
	FleetOp      *hedge.Op
	LeaderActive *timedoff.TimedOff
	LeaderId     string // valid only if `LeaderActive` is on
}
