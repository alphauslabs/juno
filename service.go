package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/alphauslabs/juno/internal/fleet"
	v1 "github.com/alphauslabs/juno/proto/v1"
	"github.com/flowerinthenight/hedge"
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

	round, err := fleet.GetLastPaxosRound(ctx, s.fd)
	glog.Infof("round=%v, err=%v", round, err)

	if fleet.IsLeader(s.fd) {
		glog.Infof("[%v] iam leader", s.fd.App.FleetOp.HostPort())
	} else {
		glog.Infof("[%v] not leader", s.fd.App.FleetOp.HostPort())
	}

	ch := make(chan hedge.BroadcastOutput)
	go s.fd.App.FleetOp.Broadcast(ctx, []byte("hello"), hedge.BroadcastArgs{Out: ch})
	for v := range ch {
		if v.Error != nil {
			glog.Errorf("err=%v", v.Error)
		} else {
			b, _ := json.Marshal(v)
			glog.Infof("out=%v", string(b))
		}
	}

	return &v1.AddToSetResponse{}, nil
}
