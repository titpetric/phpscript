package database

import (
	"context"
	"testing"
)

// TestNewBridge covers the client a script's Database holds: a statement run
// through it reaches the pool, and the observer set on it reads the query that
// ran. Every span this package records is built from that reading.
func TestNewBridge(t *testing.T) {
	pool, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	var seen []QueryLogEntry
	var bridge *Bridge = newBridge(pool)
	bridge.WithObserver(func(_ context.Context, entry QueryLogEntry) {
		seen = append(seen, entry)
	})

	if _, err := bridge.Query(t.Context(), "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if _, err := bridge.Insert(t.Context(), "t", map[string]any{"id": 7}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	row, err := bridge.Get(t.Context(), "SELECT id FROM t")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	columns, ok := row.(map[string]any)
	if !ok {
		t.Fatalf("Get() = %T, want one row as a map of its columns", row)
	}
	if got, want := columns["id"], int64(7); got != want {
		t.Errorf("Get()[id] = %v, want %v", got, want)
	}

	if len(seen) != 3 {
		t.Fatalf("the observer saw %d statements, want 3", len(seen))
	}
	if seen[0].Query != "CREATE TABLE t (id INTEGER)" {
		t.Errorf("the first statement was %q, want the CREATE TABLE", seen[0].Query)
	}
	for _, entry := range seen {
		if entry.Err != nil {
			t.Errorf("%q recorded %v, want no error", entry.Query, entry.Err)
		}
	}
}
