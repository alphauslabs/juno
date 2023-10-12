package flags

import (
	"flag"
)

var (
	Test      = flag.Bool("test", false, "Scratch pad, anything")
	Client    = flag.Bool("client", false, "Run the test client code")
	Id        = flag.String("id", "", "Node id, should be unique")
	Database  = flag.String("db", "", "Spanner database, fmt: projects/{v}/instances/{v}/databases/{v}")
	LockTable = flag.String("locktable", "juno_lock", "Spanner table for spindle lock")
	LockName  = flag.String("lockname", "juno", "Lock name for spindle")
	LogTable  = flag.String("logtable", "", "Spanner table for hedge store/log")
	GrpcPort  = flag.String("grpcport", "8080", "Port number for gRPC")
	FleetPort = flag.String("fleetport", "8081", "Port number for fleet management")
)
