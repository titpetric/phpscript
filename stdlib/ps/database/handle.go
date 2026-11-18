package database

import (
	"database/sql"
)

// DB is one connection pool, as every caller of this package holds it.
//
// DB is database/sql's pool and not a type this package declares. Named
// parameters, placeholder rebinding and struct scanning are the pdo client's
// job. It reads them off the pool at the point of use.
type DB = sql.DB

// Conn is one session held out of a pool. A PRAGMA or a transaction runs on
// one to reach the same connection twice.
type Conn = sql.Conn

// Connector opens a pool for one driver and DSN. DatabaseProvider takes one and
// a host that wants instrumented queries supplies its own.
type Connector = func(driver, dsn string) (*DB, error)

// newDB returns the pool the connector opened, with no second handle around it.
func newDB(pool *sql.DB, _ string) *DB {
	return pool
}
