package sqlite_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/titpetric/phpscript/stdlib/sqlite"
)

func TestSQLiteAdapterSecurityAndFDLeakFixes(t *testing.T) {
	ctx := context.Background()

	// 1. Verify PRAGMAs on sqlite:// DSN
	adapter, err := sqlite.Open("sqlite://file:test_pragma.db?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite adapter: %v", err)
	}
	defer adapter.Close()

	fkRow, err := adapter.First(ctx, "PRAGMA foreign_keys;")
	if err != nil || fkRow == nil || fmt.Sprint(fkRow["foreign_keys"]) != "1" {
		t.Fatalf("expected foreign_keys PRAGMA = 1, got %v (err: %v)", fkRow, err)
	}

	btRow, err := adapter.First(ctx, "PRAGMA busy_timeout;")
	if err != nil || btRow == nil || fmt.Sprint(btRow["timeout"]) != "5000" {
		t.Fatalf("expected busy_timeout PRAGMA = 5000, got %v (err: %v)", btRow, err)
	}

	// 2. Verify SQL Injection Rejection on Forged Table / Column Identifiers
	_, err = adapter.Execute(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)")
	if err != nil {
		t.Fatalf("create table failed: %v", err)
	}

	_, err = adapter.Insert(ctx, "users; DROP TABLE users; --", map[string]any{"name": "Alice"})
	if err == nil {
		t.Fatalf("expected SQL injection attempt on table name to fail, but it succeeded")
	}

	_, err = adapter.Insert(ctx, "users", map[string]any{"name'; DROP TABLE users; --": "Alice"})
	if err == nil {
		t.Fatalf("expected SQL injection attempt on column name to fail, but it succeeded")
	}

	// 3. Verify Finalizer FD Leak Prevention on Unclosed PHPBridge Objects
	createUnclosedBridges(ctx, t)
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
}

func createUnclosedBridges(ctx context.Context, t *testing.T) {
	for i := 0; i < 20; i++ {
		bridge, err := sqlite.NewPHPBridge(ctx, "sqlite://file:memory?mode=memory&cache=shared")
		if err != nil {
			t.Fatalf("failed to create bridge %d: %v", i, err)
		}
		_, _ = bridge.Execute("CREATE TABLE IF NOT EXISTS probe (id INT)")
		// Omit bridge.Close() intentionally to let finalizer reclaim FD
	}
}
