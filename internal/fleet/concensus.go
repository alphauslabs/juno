package fleet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/flags"
	"github.com/golang/glog"
)

// Phase 1a:
type Prepare struct {
	PrepareId int64 `json:"prepareId"` // proposal
	NodeId    int64 `json:"nodeId"`    // tie-breaker
	RoundNum  int64 `json:"roundNum"`  // multipaxos round number
}

// Phase 1b:
type Promise struct {
	Error          error  `json:"error"`     // non-nil = NACK
	PrepareId      int64  `json:"prepareId"` // proposal
	NodeId         int64  `json:"nodeId"`    // tie-breaker
	RoundNum       int64  `json:"roundNum"`  // multipaxos round number
	LastPrepareId  int64  `json:"lastPrepareId"`
	LastAcceptedId int64  `json:"lastAcceptedId"`
	Value          string `json:"value"`
}

// Phase 2a:
type Accept struct {
	AcceptId int64  `json:"acceptId"`
	NodeId   int64  `json:"nodeId"` // tie-breaker
	Value    string `json:"value"`
}

// Phase 2b:
type Accepted struct {
	Error    error `json:"error"` // non-nil = NACK
	AcceptId int64 `json:"acceptId"`
	NodeId   int64 `json:"nodeId"` // tie-breaker
}

// GetLastPaxosRound returns the last/current paxos round, and an indication if
// the round is already committed (false = still ongoing).
func GetLastPaxosRound(ctx context.Context, fd *FleetData) (int64, bool, error) {
	var q strings.Builder
	fmt.Fprintf(&q, "select round, updated, committed from %s ", *flags.Meta)
	fmt.Fprintf(&q, "where id = 'chain' and ((updated = committed) or (updated is not null and committed is null)) ")
	fmt.Fprintf(&q, "order by round desc limit 1")
	in := &internal.QuerySpannerSingleInput{
		Client: fd.App.Client,
		Query:  q.String(),
	}

	type roundT struct {
		Round     spanner.NullInt64
		Updated   spanner.NullTime
		Committed spanner.NullTime
	}

	rows, err := internal.QuerySpannerSingle(ctx, in)
	if err != nil {
		return 0, true, err
	}

	for _, row := range rows {
		var v roundT
		err = row.ToStruct(&v)
		if err != nil {
			return 0, true, err
		}

		glog.Infof("meta: round=%v, updated=%v, committed=%v",
			v.Round.Int64, v.Updated.Time.Format(time.RFC3339), v.Committed.Time.Format(time.RFC3339))

		return v.Round.Int64, !v.Committed.IsNull(), nil
	}

	return 0, true, nil
}
