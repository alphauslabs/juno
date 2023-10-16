package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
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
	"google.golang.org/grpc/credentials/insecure"
)

var (
	cctx = func(p context.Context) context.Context {
		return context.WithValue(p, struct{}{}, nil)
	}
)

func test() {
	i, err := strconv.Atoi("n2")
	if err != nil {
		slog.Error(err.Error())
		return
	}

	slog.Info("n2", "i", i)
}

func testClient() {
	ctx := context.Background()
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	opts = append(opts, grpc.WithBlock())
	conn, err := grpc.DialContext(ctx, "localhost:"+*flags.Client, opts...)
	if err != nil {
		slog.Error("fail to dial:", "err", err)
		return
	}

	defer conn.Close()
	client := v1.NewJunoClient(conn)
	out, err := client.AddToSet(ctx, &v1.AddToSetRequest{
		Key:   "set1",
		Value: "val1",
	})

	if err != nil {
		slog.Error("AddToSet failed:", "err", err)
		return
	}

	slog.Info("out:", "count", out.Count)
}

func grpcServe(ctx context.Context, fd *fleet.FleetData, done chan error) error {
	l, err := net.Listen("tcp", ":"+*flags.GrpcPort)
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

	svc := &service{fd: fd}
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

	if *flags.Client != "" {
		testClient()
		return
	}

	// Test:
	if *flags.Test {
		test()
		return
	}

	if *flags.Id < 1 {
		glog.Errorf("invalid id [%v]", *flags.Id)
		return
	}

	app := &appdata.AppData{}
	var err error
	ctx, cancel := context.WithCancel(context.Background())
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
		"", // not using hedge's logtable
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
			glog.Infof("%v, took %v", *line, time.Since(begin))
		}(&m, time.Now())

		glog.Infof("attempt leader wait...")
		ok, err := fleet.EnsureLeaderActive(cctx(ctx), app)
		switch {
		case !ok:
			m = fmt.Sprintf("failed: %v, no leader after", err)
		default:
			m = "confirm leader active"
		}
	}()

	go fleet.LeaderLiveness(cctx(ctx), app)

	// For debug: log if no leader detected.
	app.LeaderActive = timedoff.New(time.Second*5, &timedoff.CallbackT{
		Callback: func(args interface{}) {
			glog.Infof("no leader for the past 5s?")
			atomic.StoreInt64(&app.LeaderId, 0) // set no leader
		},
	})

	// Setup our gRPC management API.
	go func() {
		glog.Infof("serving grpc at :%v", *flags.GrpcPort)
		if err := grpcServe(ctx, &fleetData, done); err != nil {
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
