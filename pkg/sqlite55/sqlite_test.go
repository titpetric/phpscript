package sqlite55_test

import (
	"context"
	"testing"

	"github.com/titpetric/phpscript/pkg/sqlite55"
)

func TestSQLiteAdapterMemoryOperations(t *testing.T) {
	ctx := context.Background()

	adapter, err := sqlite55.Memory()
	if err != nil {
		t.Fatalf("failed to open memory sqlite adapter: %v", err)
	}
	defer adapter.Close()

	// Execute DDL
	_, err = adapter.Execute(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, email TEXT)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Insert
	id, err := adapter.Insert(ctx, "users", map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if id != 1 {
		t.Fatalf("expected inserted id 1, got %d", id)
	}

	// First
	user, err := adapter.First(ctx, "SELECT * FROM users WHERE id = ?", id)
	if err != nil {
		t.Fatalf("first query failed: %v", err)
	}
	if user == nil || user["name"] != "Alice" {
		t.Fatalf("expected user name Alice, got %v", user)
	}

	// All
	users, err := adapter.All(ctx, "SELECT * FROM users")
	if err != nil || len(users) != 1 {
		t.Fatalf("all query failed or expected 1 row, got %d", len(users))
	}

	// Update
	affected, err := adapter.Update(ctx, "users", map[string]any{"name": "Alice Smith"}, "id = ?", id)
	if err != nil || affected != 1 {
		t.Fatalf("update failed: affected %d, err %v", affected, err)
	}

	// Delete
	affected, err = adapter.Delete(ctx, "users", "id = ?", id)
	if err != nil || affected != 1 {
		t.Fatalf("delete failed: affected %d, err %v", affected, err)
	}
}
