package main

import (
	"context"

	"github.com/alphauslabs/juno/internal/appdata"
	v1 "github.com/alphauslabs/juno/proto/v1"
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
	return &v1.AddToSetResponse{}, nil
}
