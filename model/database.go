package model

import (
	"context"
	"database/sql"
)

// DatabaseProvider resolves the named connections a script opens.
//
// The interface lives here so a runtime can name it among its options without
// depending on the implementing package: stdlib/ps/database. That package
// imports the runtime to register its bindings. A runtime knows only that
// something answers to a connection name; which connections exist is the host's
// business. A virtual host answers differently from the process it shares.
type DatabaseProvider interface {
	// Open returns a client for the first name the host configured.
	Open(ctx context.Context, names ...string) (*sql.DB, error)

	// Connect is Open plus a ping: the ping proves the storage reachable
	// before the caller sees a client.
	Connect(ctx context.Context, names ...string) (*sql.DB, error)
}

// ExtendedDatabaseProvider extends database providers with listing and registration.
type ExtendedDatabaseProvider interface {
	List() []string
	Register(string, string)
}
