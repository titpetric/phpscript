package model

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// DatabaseProvider resolves the named connections a script opens.
//
// The interface lives here so a runtime can name it in its options without
// depending on the package that implements it, which is stdlib/database and
// which imports the runtime to register its bindings. A runtime knows only that
// something answers to a connection name; which connections exist is the host's
// business, and a virtual host answers differently from the process it shares.
type DatabaseProvider interface {
	// Open returns a client for the first name that is configured.
	Open(ctx context.Context, names ...string) (*sqlx.DB, error)

	// Connect is Open plus a ping, so the caller knows the storage is
	// reachable before it is handed a client.
	Connect(ctx context.Context, names ...string) (*sqlx.DB, error)
}

// ExtendedDatabaseProvider extends database providers with listing and registration.
type ExtendedDatabaseProvider interface {
	List() []string
	Register(string, string)
}
