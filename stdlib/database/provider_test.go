package database

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestDatabaseProviderConnect(t *testing.T) {
	provider := NewDatabaseProvider(sqlx.Open)
	provider.Register("test", "sqlite://:memory:")

	db, err := provider.Connect(t.Context(), "test")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if db == nil {
		t.Fatal("Connect returned a nil database")
	}

	db2, err := provider.Connect(t.Context(), "test")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if db2 != db {
		t.Errorf("Connect returned %p, want the cached %p", db2, db)
	}
}

func TestDatabaseProviderOpenError(t *testing.T) {
	provider := NewDatabaseProvider(func(string, string) (*sqlx.DB, error) {
		return nil, errors.New("test")
	})
	provider.Register("test", "sqlite://:memory:")

	db, err := provider.Open(t.Context(), "test")
	if err == nil {
		t.Fatal("Open: want an error from the open function")
	}
	if db != nil {
		t.Errorf("Open returned %p, want nil on error", db)
	}
}

func TestDatabaseProviderFileSQLiteDefaults(t *testing.T) {
	provider := NewDatabaseProvider(sqlx.Open)
	provider.Register("test", "sqlite://"+filepath.Join(t.TempDir(), "test.db"))

	db, err := provider.Connect(t.Context(), "test")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.Stats().MaxOpenConnections; got != 10 {
		t.Errorf("MaxOpenConnections = %d, want 10", got)
	}

	first, err := db.Connx(t.Context())
	if err != nil {
		t.Fatalf("Connx: %v", err)
	}
	defer first.Close()
	second, err := db.Connx(t.Context())
	if err != nil {
		t.Fatalf("Connx: %v", err)
	}
	defer second.Close()

	for _, connection := range []*sqlx.Conn{first, second} {
		var journalMode string
		if err := connection.GetContext(t.Context(), &journalMode, "PRAGMA journal_mode"); err != nil {
			t.Fatalf("PRAGMA journal_mode: %v", err)
		}
		if journalMode != "wal" {
			t.Errorf("journal_mode = %q, want wal", journalMode)
		}

		var busyTimeout int
		if err := connection.GetContext(t.Context(), &busyTimeout, "PRAGMA busy_timeout"); err != nil {
			t.Fatalf("PRAGMA busy_timeout: %v", err)
		}
		if busyTimeout != 5000 {
			t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
		}
	}
}

// TestNewReadsPlatformDBEnvironment covers the environment form a host
// configures connections in: a PLATFORM_DB_<NAME> entry becomes a lowercase
// connection name, alongside the built-in default.
func TestNewReadsPlatformDBEnvironment(t *testing.T) {
	provider := New([]string{"PLATFORM_DB_SHOP=sqlite://:memory:"})

	got := provider.List()
	slices.Sort(got)

	want := []string{"default", "shop"}
	if !slices.Equal(got, want) {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

// TestNewIsolatesProviders is the property virtual hosting rests on: a
// provider only resolves the connections it was built with, and two
// providers do not share the databases they open.
func TestNewIsolatesProviders(t *testing.T) {
	shop := New([]string{"PLATFORM_DB_SHOP=sqlite://:memory:"})
	blog := New([]string{"PLATFORM_DB_BLOG=sqlite://:memory:"})

	if _, err := shop.Open(t.Context(), "shop"); err != nil {
		t.Fatalf("shop.Open(shop): %v", err)
	}

	_, err := blog.Open(t.Context(), "shop")
	if err == nil {
		t.Fatal("blog.Open(shop): want an error, the connection is not the blog's")
	}
	if !strings.Contains(err.Error(), "no configuration found for database") {
		t.Errorf("blog.Open(shop) error = %v, want a missing configuration error", err)
	}

	shopDefault, err := shop.Open(t.Context())
	if err != nil {
		t.Fatalf("shop.Open(): %v", err)
	}
	blogDefault, err := blog.Open(t.Context())
	if err != nil {
		t.Fatalf("blog.Open(): %v", err)
	}
	if shopDefault == blogDefault {
		t.Error("both providers returned the same default database, want one each")
	}
}
