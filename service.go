package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/flags"
	"github.com/alphauslabs/juno/internal/fleet"
	pb "github.com/alphauslabs/juno/proto/v1"
	"github.com/golang/glog"
	gaxv2 "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type service struct {
	fd *fleet.FleetData
	pb.UnimplementedJunoServer
}

func (s *service) Lock(stream pb.Juno_LockServer) error {
	return nil
}

func (s *service) AddToSet(ctx context.Context, req *pb.AddToSetRequest) (*pb.AddToSetResponse, error) {
	if atomic.LoadInt32(&s.fd.AddToSetReady) == 0 {
		m := fmt.Sprintf("Node %v not yet ready. Please try again later.", *flags.Id)
		return nil, status.Errorf(codes.Unavailable, m)
	}

	defer func(begin time.Time) { glog.Infof("method AddToSet took %v", time.Since(begin)) }(time.Now())

	reqb, _ := json.Marshal(req)
	glog.Infof("request=%v", string(reqb))

	var attempts int
	bo := gaxv2.Backoff{} // 30s
	for {
		if !fleet.NoLeader(s.fd) {
			break
		} else {
			attempts++
			if attempts >= 10 {
				return nil, status.Errorf(codes.Unavailable, fleet.ErrNoLeader.Error())
			}

			time.Sleep(bo.Pause())
			continue
		}
	}

	var out *fleet.ReachConsensusOutput
	var err error

	switch {
	case fleet.IsLeader(s.fd):
		out, err = fleet.ReachConsensus(ctx, &fleet.ReachConsensusInput{
			FleetData: s.fd,
			CmdType:   fleet.CmdTypeAddToSet,
			Key:       req.Key,
			Value:     req.Value,
		})

		if err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}

	default:
		b, _ := json.Marshal(internal.NewEvent(
			fleet.ReachConsensusInput{
				CmdType: fleet.CmdTypeAddToSet,
				Key:     req.Key,
				Value:   req.Value,
			},
			fleet.EventSource,
			fleet.CtrlLeaderFwdConsensus,
		))

		outb, err := fleet.SendToLeader(ctx, s.fd.App, b)
		if err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}

		json.Unmarshal(outb, &out)
	}

	limit := 5   // 5s max retry limit
	attempts = 0 // reset
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		s.fd.SetValueMtx.Lock()
		copy, ok := s.fd.SetValueHistory[int(out.Round)]
		s.fd.SetValueMtx.Unlock()
		if ok && copy.Applied {
			break
		}

		time.Sleep(time.Millisecond * 1)
		attempts++
		if attempts > 0 && (attempts%1000 == 0) {
			glog.Infof("checkpoint: Cannot get value for round %v.", out.Round)
			limit--
			if limit <= 0 {
				return nil, status.Errorf(codes.Internal, "Cannot get value for round %v.", out.Round)
			}
		}
	}

	count := len(s.fd.StateMachine.Members(req.Key))
	return &pb.AddToSetResponse{Key: out.Key, Count: int64(count)}, nil
}
