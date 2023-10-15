package main

import (
	"context"
	"encoding/json"

	"github.com/alphauslabs/juno/internal/appdata"
	v1 "github.com/alphauslabs/juno/proto/v1"
	"github.com/flowerinthenight/hedge"
	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type service struct {
	app *appdata.AppData

	v1.UnimplementedJunoServer
}

func (s *service) Lock(in *v1.LockRequest, stream v1.Juno_LockServer) error {
	return status.Errorf(codes.Unimplemented, "method Lock not implemented")
}

func (s *service) Unlock(ctx context.Context, in *v1.UnlockRequest) (*v1.UnlockResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Unlock not implemented")
}

func (s *service) AddToSet(ctx context.Context, in *v1.AddToSetRequest) (*v1.AddToSetResponse, error) {
	ch := make(chan hedge.BroadcastOutput)
	go s.app.FleetOp.Broadcast(ctx, []byte("hello"), hedge.BroadcastArgs{Out: ch})
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
