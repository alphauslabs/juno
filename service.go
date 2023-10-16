package main

import (
	"context"
	"encoding/json"
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
	bo := gaxv2.Backoff{Max: time.Minute}
	for {
		if !fleet.NoLeader(s.fd) {
			break
		} else {
			attempts++
			if attempts >= 10 {
				return nil, status.Errorf(codes.Unavailable, "Leader unavailable. Please try again later.")
			}

			time.Sleep(bo.Pause())
			continue
		}
	}

	switch {
	case fleet.IsLeader(s.fd):
		out, err := fleet.StartPaxos(ctx, &fleet.StartPaxosInput{
			FleetData: s.fd,
			Key:       req.Key,
			Value:     req.Value,
		})

		outb, _ := json.Marshal(out)
		glog.Infof("direct: out=%v, err=%v", string(outb), err)
	default:
		b, _ := json.Marshal(internal.NewEvent(
			fleet.StartPaxosInput{
				Key:   req.Key,
				Value: req.Value,
			},
			fleet.EventSource,
			fleet.CtrlLeaderFwdPaxos,
		))

		outb, err := fleet.SendToLeader(ctx, s.fd.App, b)
		glog.Infof("fwd: out=%v, err=%v", string(outb), err)
	}

	return &v1.AddToSetResponse{}, nil
}
