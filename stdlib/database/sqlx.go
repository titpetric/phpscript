package database

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// DB is one connection pool, as every caller of this package holds it.
//
// DB is sqlx's handle and not a wrapper this package declares: the named
// parameters and the struct scanning are the whole reason for the dependency,
// and a type that hid them would declare them again under other names.
type DB = sqlx.DB

// Conn is one session held out of a pool. A PRAGMA or a transaction runs on
// one to reach the same connection twice.
type Conn = sqlx.Conn

// Connector opens a pool for one driver and DSN. DatabaseProvider takes one, so
// a host that wants instrumented queries supplies its own.
type Connector = func(driver, dsn string) (*DB, error)

// newDB wraps an open database/sql pool, under the driver name the handle reads
// to pick the bind variable form.
func newDB(pool *sql.DB, driver string) *DB {
	return sqlx.NewDb(pool, driver)
}
