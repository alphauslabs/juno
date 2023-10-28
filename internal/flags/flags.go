package flags

import (
	"flag"
	"os"
)

var (
	Test      = flag.Bool("test", false, "Scratch pad, anything")
	Client    = flag.String("client", "", "Run the test client code, fmt: [lock:]<port[,key,val,{loopnum}]>")
	Id        = flag.Int("id", 0, "Node id, should be a number, unique, and greater than 0")
	NodeCount = flag.Int("nodecount", 3, "Expected number of nodes in the fleet")
	Database  = flag.String("db", "", "Spanner database, fmt: projects/{v}/instances/{v}/databases/{v}")
	LockTable = flag.String("locktable", "juno_lock", "Spanner table for spindle lock")
	LockName  = flag.String("lockname", "juno", "Lock name for spindle")
	Meta      = flag.String("meta", "juno_meta", "Spanner table for metadata")
	GrpcPort  = flag.String("grpcport", "8080", "Port number for gRPC")
	FleetPort = flag.String("fleetport", "8081", "Port number for fleet management")
	Slack     = flag.String("slack", os.Getenv("SLACK_TRACEME"), "Slack endpoint for notifications")
)
