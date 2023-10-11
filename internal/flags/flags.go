package flags

import (
	"flag"
)

var (
	Test      = flag.Bool("test", false, "Scratch pad, anything")
	Id        = flag.String("id", "", "Node id, should be unique")
	Database  = flag.String("db", "", "Spanner database, fmt: projects/{v}/instances/{v}/databases/{v}")
	LockTable = flag.String("locktable", "juno_lock", "Spanner table for spindle lock")
	LockName  = flag.String("lockname", "juno", "Lock name for spindle")
	LogTable  = flag.String("logtable", "juno_store", "Spanner table for hedge store/log")
)
