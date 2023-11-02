package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	if atomic.LoadInt32(&s.fd.Online) == 0 {
		m := fmt.Sprintf("Node %v not yet ready. Please try again later.", *flags.Id)
		return status.Errorf(codes.Unavailable, m)
	}

	defer func(begin time.Time) { glog.Infof("method Lock took %v", time.Since(begin)) }(time.Now())

	ctx := stream.Context()
	var attempts int
	bo := gaxv2.Backoff{} // 30s
	for {
		if !fleet.NoLeader(s.fd) {
			break
		} else {
			attempts++
			if attempts >= 10 {
				return status.Errorf(codes.Unavailable, fleet.ErrNoLeader.Error())
			}

			time.Sleep(bo.Pause())
			continue
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}

		b, _ := json.Marshal(req)
		glog.Infof("stream req: %v", string(b))

		if err != nil {
			glog.Errorf("Recv failed: %v", err)
			return err
		}

		switch {
		case req.Name != "" && !req.Release && !req.Extend: // acquire lock
			var available bool
			lockState := s.fd.StateMachine.GetLockState(req.Name) // from local copy
			if lockState.Name != "" && lockState.State == 0 {
				available = true
			}

			if available {
				var duration int64 = 5
				if req.LeaseTime > duration {
					duration = req.LeaseTime
				}

				b, _ := json.Marshal(internal.NewEvent(
					fleet.ReachConsensusInput{
						CmdType: fleet.CmdTypeGetLock,
						Key:     req.Name,                    // lock name
						Value:   fmt.Sprintf("%v", duration), // lease duration
					},
					fleet.EventSource,
					fleet.CtrlLeaderFwdConsensus,
				))

				outb, err := fleet.SendToLeader(ctx, s.fd.App, b)
				if err != nil {
					return status.Errorf(codes.Internal, err.Error())
				}

				var out *fleet.ReachConsensusOutput
				json.Unmarshal(outb, &out)

				limit := 5   // 5s max retry limit
				attempts = 0 // reset
				for {
					if ctx.Err() != nil {
						return ctx.Err()
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
							return status.Errorf(codes.Internal, "Cannot get value for round %v.", out.Round)
						}
					}
				}

				lockState = s.fd.StateMachine.GetLockState(req.Name) // from local copy
				if lockState.State == 1 {
					resp := pb.LockResponse{
						Name:  req.Name,
						Token: "token",
					}

					if err := stream.Send(&resp); err != nil {
						glog.Errorf("Send failed: %v", err)
						return err
					}
				}
			} else {
				return status.Errorf(codes.Unavailable, "Lock unavailable. Add wait option (todo).")
			}
		}
	}
}

func (s *service) AddToSet(ctx context.Context, req *pb.AddToSetRequest) (*pb.AddToSetResponse, error) {
	if atomic.LoadInt32(&s.fd.Online) == 0 {
		m := fmt.Sprintf("Node %v not yet ready. Please try again later.", *flags.Id)
		return nil, status.Errorf(codes.Unavailable, m)
	}

	var leader int
	var attempts int
	defer func(begin time.Time, l, a *int) {
		glog.Infof("method AddToSet (leader?=%v, attempts=%v) took %v", *l, *a, time.Since(begin))
	}(time.Now(), &leader, &attempts)

	reqb, _ := json.Marshal(req)
	glog.Infof("request=%v", string(reqb))

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
		leader = 1
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
