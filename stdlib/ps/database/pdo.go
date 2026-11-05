package database

import (
	"github.com/titpetric/pdo/client"
)

// Bridge is the pdo client a Database holds. Every statement a script runs is
// one of its methods; this package adds the tracing around them and the
// read-only refusal in front of them.
type Bridge = client.Bridge

// QueryLogEntry is one statement the bridge ran, as its observer reads it.
type QueryLogEntry = client.QueryLogEntry

// newBridge opens a client over a pool. The caller sets the observer, because
// the observer a Database installs is one of its own methods.
func newBridge(pool *DB) *Bridge {
	return client.NewBridge(pool)
}
