package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	rsm := make(map[string]map[string]struct{})
	rsm["1"] = make(map[string]struct{})
	rsm["1"]["1"] = struct{}{}
	rsm["1"]["2"] = struct{}{}
	rsm["1"]["3"] = struct{}{}
	rsm["1"]["3"] = struct{}{}
	slog.Info("len:", "root", len(rsm), "1", len(rsm["1"]))

	copy := make(map[string]map[string]struct{})
	for k, v := range rsm {
		copy[k] = v
	}

	slog.Info("copy:", "val", copy)
}

func testClient() {
	ctx := context.Background()
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	opts = append(opts, grpc.WithBlock())
	ss := strings.Split(*flags.Client, ",")
	key, val := "set1", "val1"
	if len(ss) == 2 {
		key = ss[1]
	}

	if len(ss) == 3 {
		key = ss[1]
		val = ss[2]
	}

	conn, err := grpc.DialContext(ctx, "localhost:"+ss[0], opts...)
	if err != nil {
		slog.Error("fail to dial:", "err", err)
		return
	}

	defer conn.Close()
	client := v1.NewJunoClient(conn)

	if len(ss) == 4 {
		slog.Info("start...")
		key = ss[1]
		val = ss[2]

		n, err := strconv.Atoi(ss[3])
		if err != nil {
			slog.Error(err.Error())
			return
		}

		for i := 0; i < n; i++ {
			time.Sleep(time.Millisecond * 100)
			out, err := client.AddToSet(ctx, &v1.AddToSetRequest{
				Key:   key,
				Value: val,
			})

			if err != nil {
				slog.Error("AddToSet failed:", "err", err)
				continue
			}

			outb, _ := json.Marshal(out)
			slog.Info("out:", "val", string(outb))
		}
	} else {
		out, err := client.AddToSet(ctx, &v1.AddToSetRequest{
			Key:   key,
			Value: val,
		})

		if err != nil {
			slog.Error("AddToSet failed:", "err", err)
			return
		}

		outb, _ := json.Marshal(out)
		slog.Info("out:", "val", string(outb))
	}
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
		// See if we are deployed as statefulset.
		hn, _ := os.Hostname()
		ss := strings.Split(hn, "-")
		if len(ss) == 2 && ss[0] == "juno" {
			n, _ := strconv.ParseInt(ss[1], 10, 64)
			*flags.Id = int(n + 1)
		}
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
	fleetData := fleet.FleetData{
		App:          app,
		StateMachine: fleet.NewRsm(),
	}

	fleetData.BuildRsmWip = timedoff.New(time.Second*2, &timedoff.CallbackT{
		Callback: func(args interface{}) {
			atomic.StoreInt64(&fleetData.BuildRsmOn, 0)
		},
	})

	// Setup our group coordinator.
	app.FleetOp = hedge.New(
		app.Client,
		":"+*flags.FleetPort,
		*flags.LockTable,
		*flags.LockName,
		"", // not using hedge's logtable
		hedge.WithGroupSyncInterval(time.Second*5),
		hedge.WithLeaderHandler(&fleetData, fleet.LeaderHandler),
		hedge.WithBroadcastHandler(&fleetData, fleet.BroadcastHandler),
		hedge.WithLogger(log.New(io.Discard, "", 0)),
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

	// Build our replicated state machine.
	fleet.BuildRsm(cctx(ctx), &fleetData)

	// Monitor any drift in our replicated log.
	go fleet.MonitorRsmDrift(cctx(ctx), &fleetData)

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
