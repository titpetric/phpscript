package database

import (
	"github.com/titpetric/pdo/client"
)

// Bridge is the pdo client a Database holds. Every statement a script runs
// calls a bridge method; this package adds the tracing around those methods and
// the read-only refusal in front of them.
type Bridge = client.Bridge

// QueryLogEntry holds a statement the bridge ran, as its observer reads it.
type QueryLogEntry = client.QueryLogEntry

// newBridge opens a client over pool, leaving the observer to the caller.
func newBridge(pool *DB) *Bridge {
	return client.NewBridge(pool)
}
