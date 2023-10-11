package appdata

import (
	"cloud.google.com/go/spanner"
	"github.com/flowerinthenight/hedge"
	"github.com/flowerinthenight/timedoff"
)

type AppData struct {
	Client       *spanner.Client
	FleetOp      *hedge.Op
	LeaderActive *timedoff.TimedOff
}
