package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/alphauslabs/juno/internal/appdata"
	"github.com/alphauslabs/juno/internal/flags"
	"github.com/alphauslabs/juno/internal/fleet"
	rl "github.com/alphauslabs/juno/internal/ratelimit"
	v1 "github.com/alphauslabs/juno/proto/v1"
	"github.com/flowerinthenight/hedge"
	"github.com/flowerinthenight/timedoff"
	"github.com/golang/glog"
	"github.com/grpc-ecosystem/go-grpc-middleware/ratelimit"
	"google.golang.org/grpc"
)

var (
	cctx = func(p context.Context) context.Context {
		return context.WithValue(p, struct{}{}, nil)
	}
)

func grpcServe(ctx context.Context, network, port string, done chan error) error {
	l, err := net.Listen(network, ":"+port)
	if err != nil {
		glog.Errorf("net.Listen failed: %v", err)
		return err
	}

	defer l.Close()
	gs := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			ratelimit.UnaryServerInterceptor(&rl.Limiter{}),
		),
		grpc.ChainStreamInterceptor(
			ratelimit.StreamServerInterceptor(&rl.Limiter{}),
		),
	)

	svc := &service{}
	v1.RegisterJunoServer(gs, svc)

	go func() {
		<-ctx.Done()
		gs.GracefulStop()
		done <- nil
	}()

	return gs.Serve(l)
}

func main() {
	flag.Parse()
	defer glog.Flush()

	// Test:
	if *flags.Test {
		log.Println("todo")
		return
	}

	app := &appdata.AppData{}
	var err error
	ctx, cancel := context.WithCancel(context.Background())

	// For debug: log if no leader detected.
	app.LeaderActive = timedoff.New(time.Minute*30, &timedoff.CallbackT{
		Callback: func(args interface{}) {
			glog.Errorf("failed: no leader for the past 30mins?")
		},
	})

	app.Client, err = spanner.NewClient(cctx(ctx), *flags.Database)
	if err != nil {
		glog.Fatal(err) // essential
	}

	defer app.Client.Close()
	fleetData := fleet.FleetData{App: app}

	// Setup our group coordinator.
	app.FleetOp = hedge.New(
		app.Client,
		":"+*flags.FleetPort,
		*flags.LockTable,
		*flags.LockName,
		*flags.LogTable,
		hedge.WithGroupSyncInterval(time.Second*10),
		hedge.WithLeaderHandler(&fleetData, fleet.LeaderHandler),
		hedge.WithBroadcastHandler(&fleetData, fleet.BroadcastHandler),
	)

	done := make(chan error)
	doneLock := make(chan error, 1)
	go app.FleetOp.Run(cctx(ctx), doneLock)

	// Ensure leader is active before proceeding.
	func() {
		var m string
		defer func(line *string, begin time.Time) {
			glog.Infof("%v %v", *line, time.Since(begin))
		}(&m, time.Now())

		glog.Infof("attempt leader wait...")
		ok, err := fleet.EnsureLeaderActive(cctx(ctx), app)
		switch {
		case !ok:
			m = fmt.Sprintf("failed: %v, no leader after", err)
		default:
			m = "confirm: leader active after"
		}
	}()

	go fleet.LeaderLiveness(cctx(ctx), app)

	// Setup our gRPC management API.
	go func() {
		port := *flags.GrpcPort
		glog.Infof("serving grpc at :%v", port)
		if err := grpcServe(ctx, "tcp", port, done); err != nil {
			glog.Fatal(err)
		}
	}()

	// Interrupt handler.
	go func() {
		sigch := make(chan os.Signal)
		signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
		glog.Infof("signal: %v", <-sigch)
		cancel()
	}()

	<-done
	<-doneLock
}
