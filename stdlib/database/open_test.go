package database

import (
	"testing"
)

// TestOpen covers the pool DatabaseProvider builds by default. sql.Open
// records the driver and the DSN and connects to nothing and a pool exists
// before any server has answered, and a provider holding ten connection
// strings opens no sockets at startup. The sqlx wrapper keeps the driver
// name, and sqlx reads it there to rebind placeholders for the dialect.
func TestOpen(t *testing.T) {
	t.Parallel()
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.DriverName(); got != "sqlite" {
		t.Errorf("DriverName() = %q, want %q", got, "sqlite")
	}
	if got := db.Stats().OpenConnections; got != 0 {
		t.Errorf("OpenConnections = %d before the first statement, want 0", got)
	}

	// The pool is usable: the first statement is what dials.
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("PingContext: %v", err)
	}
	if got := db.Stats().OpenConnections; got != 1 {
		t.Errorf("OpenConnections = %d after a ping, want 1", got)
	}
}

// newLiveDatabase returns a client over an in-memory sqlite holding a users
// table with two rows. It holds the pool to one connection, because a second
// connection to ":memory:" opens a second database and would find no table.
// The blank import in provider_test.go registers the driver, and it compiles
// into this same test binary.
func newLiveDatabase(t *testing.T) *Database {
	t.Helper()

	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	for _, statement := range []string{
		"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO users (id, name) VALUES (1, 'Ada'), (2, 'Grace')",
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	return &Database{Bridge: newBridge(db)}
}
