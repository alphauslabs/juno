package main

import (
	v1 "github.com/alphauslabs/juno/proto/v1"
)

type service struct {
	v1.UnimplementedJunoServer
}
