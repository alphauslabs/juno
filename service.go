package main

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/alphauslabs/juno/internal"
	"github.com/alphauslabs/juno/internal/fleet"
	v1 "github.com/alphauslabs/juno/proto/v1"
	"github.com/golang/glog"
	gaxv2 "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type service struct {
	fd *fleet.FleetData
	v1.UnimplementedJunoServer
}

func (s *service) Lock(in *v1.LockRequest, stream v1.Juno_LockServer) error {
	return status.Errorf(codes.Unimplemented, "method Lock not implemented")
}

func (s *service) Unlock(ctx context.Context, req *v1.UnlockRequest) (*v1.UnlockResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Unlock not implemented")
}

func (s *service) AddToSet(ctx context.Context, req *v1.AddToSetRequest) (*v1.AddToSetResponse, error) {
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

	switch {
	case fleet.IsLeader(s.fd):
		out, err := fleet.ReachConsensus(ctx, &fleet.ReachConsensusInput{
			FleetData: s.fd,
			CmdType:   fleet.CmdTypeAddToSet,
			Key:       req.Key,
			Value:     req.Value,
		})

		if err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}

		return &v1.AddToSetResponse{
			Key:   out.Key,
			Count: int64(out.Count),
		}, nil
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

		sigval := atomic.LoadInt64(&s.fd.SignalSetValue)
		outb, err := fleet.SendToLeader(ctx, s.fd.App, b)
		if err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}

		var out fleet.ReachConsensusOutput
		err = json.Unmarshal(outb, &out)
		if err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}

		// Naive way of telling we received the SetValue broadcast.
		var attempts int
		for {
			sigval2 := atomic.LoadInt64(&s.fd.SignalSetValue)
			if sigval2 != sigval {
				break
			} else {
				time.Sleep(time.Millisecond * 1)
				attempts++
				if attempts >= 500 { // 500ms
					return nil, status.Errorf(codes.Internal, "Cannot get value.")
				}
			}
		}

		count := len(s.fd.StateMachine.Members(req.Key))
		return &v1.AddToSetResponse{
			Key:   out.Key,
			Count: int64(count),
		}, nil
	}
}
