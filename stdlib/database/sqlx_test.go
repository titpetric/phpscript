package database

import (
	"database/sql"
	"testing"
)

// TestNewDB covers the handle every binding of this package queries through:
// it carries the driver name the bind variable form is picked from, it keeps
// the pool it was built on, and a query runs on it.
func TestNewDB(t *testing.T) {
	pool, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	db := newDB(pool, "sqlite")
	if got := db.DriverName(); got != "sqlite" {
		t.Errorf("DriverName() = %q, want sqlite", got)
	}
	if db.DB != pool {
		t.Error("the handle wraps a different pool than it was built on")
	}

	var answer int
	if err := db.GetContext(t.Context(), &answer, "SELECT 1"); err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if answer != 1 {
		t.Errorf("SELECT 1 = %d, want 1", answer)
	}
}

// TestConnector covers the open function a provider is built with. Open
// satisfies it, which is what makes the default provider and an instrumented
// one interchangeable.
func TestConnector(t *testing.T) {
	var connector Connector = Open

	db, err := connector("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("connector: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	connection, err := db.Connx(t.Context())
	if err != nil {
		t.Fatalf("Connx: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	var one int
	if err := connection.GetContext(t.Context(), &one, "SELECT 1"); err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if one != 1 {
		t.Errorf("SELECT 1 = %d, want 1", one)
	}
}
