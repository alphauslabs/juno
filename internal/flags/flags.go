package flags

import (
	"flag"
)

var (
	Test      = flag.Bool("test", false, "Scratch pad, anything")
	Client    = flag.String("client", "", "Run the test client code, set value to the port to use")
	Id        = flag.Int("id", 0, "Node id, should be a number, unique, and greater than 0")
	NodeCount = flag.Int("nodecount", 3, "Expected number of nodes in the fleet")
	Database  = flag.String("db", "", "Spanner database, fmt: projects/{v}/instances/{v}/databases/{v}")
	LockTable = flag.String("locktable", "juno_lock", "Spanner table for spindle lock")
	LockName  = flag.String("lockname", "juno", "Lock name for spindle")
	Meta      = flag.String("meta", "juno_meta", "Spanner table for metadata")
	GrpcPort  = flag.String("grpcport", "8080", "Port number for gRPC")
	FleetPort = flag.String("fleetport", "8081", "Port number for fleet management")
)
