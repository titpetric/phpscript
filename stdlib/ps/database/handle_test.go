package database

import (
	"database/sql"
	"testing"
)

// TestNewDB covers the handle every binding of this package queries through:
// it is the pool the connector opened and a query runs on it.
func TestNewDB(t *testing.T) {
	t.Parallel()
	pool, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	db := newDB(pool, "sqlite")
	if db != pool {
		t.Error("the handle wraps a different pool than it was built on")
	}

	var answer int
	if err := db.QueryRowContext(t.Context(), "SELECT 1").Scan(&answer); err != nil {
		t.Fatalf("QueryRowContext: %v", err)
	}
	if answer != 1 {
		t.Errorf("SELECT 1 = %d, want 1", answer)
	}
}

// TestConnector covers the open function a provider takes. Open satisfies it.
// A host builds the default provider and an instrumented one the same way.
func TestConnector(t *testing.T) {
	t.Parallel()
	var connector Connector = Open

	db, err := connector("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("connector: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	connection, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	var one int
	if err := connection.QueryRowContext(t.Context(), "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("QueryRowContext: %v", err)
	}
	if one != 1 {
		t.Errorf("SELECT 1 = %d, want 1", one)
	}
}
