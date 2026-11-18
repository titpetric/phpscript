package database

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestProvider covers the alias a host names in its runtime options. model
// declares the interface. A runtime can name a provider without depending on
// this package. The alias keeps one shared name. A runtime whose options name
// none resolves through Default, which supplies the process environment in a
// CLI run. A connection resolves before a host has configured anything.
func TestProvider(t *testing.T) {
	t.Parallel()
	var provider Provider = New([]string{"PLATFORM_DB_SHOP=sqlite://:memory:"})

	db, err := provider.Open(t.Context(), "shop")
	if err != nil {
		t.Fatalf("Open(shop) through the interface: %v", err)
	}
	if db == nil {
		t.Fatal("Open returned a nil pool")
	}

	if Default == nil {
		t.Fatal("Default holds no provider, so a CLI run resolves nothing")
	}
	if _, err := Default.Open(t.Context()); err != nil {
		t.Errorf("Default.Open() = %v, want the built-in default connection", err)
	}
}

func TestDatabaseProviderConnect(t *testing.T) {
	t.Parallel()
	provider := NewDatabaseProvider(Open)
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
	t.Parallel()
	provider := NewDatabaseProvider(func(string, string) (*DB, error) {
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

func TestDatabaseProviderNewAt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	provider := NewAt(root, []string{"PLATFORM_DB_APP=sqlite://app.db"})

	db, err := provider.Connect(t.Context(), "app")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(t.Context(), "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	// The write landed under root. The working directory did not receive it.
	if _, err := os.Stat(filepath.Join(root, "app.db")); err != nil {
		t.Errorf("Stat: %v, want app.db under the provider root", err)
	}
}

func TestDatabaseProviderFileSQLiteDefaults(t *testing.T) {
	t.Parallel()
	provider := NewDatabaseProvider(Open)
	provider.Register("test", "sqlite://"+filepath.Join(t.TempDir(), "test.db"))

	db, err := provider.Connect(t.Context(), "test")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.Stats().MaxOpenConnections; got != 10 {
		t.Errorf("MaxOpenConnections = %d, want 10", got)
	}

	first, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer first.Close()
	second, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer second.Close()

	for _, connection := range []*Conn{first, second} {
		var journalMode string
		if err := connection.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
			t.Fatalf("PRAGMA journal_mode: %v", err)
		}
		if journalMode != "wal" {
			t.Errorf("journal_mode = %q, want wal", journalMode)
		}

		var busyTimeout int
		if err := connection.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
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
	t.Parallel()
	provider := New([]string{"PLATFORM_DB_SHOP=sqlite://:memory:"})

	got := provider.List()
	slices.Sort(got)

	want := []string{"default", "shop"}
	if !slices.Equal(got, want) {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

// TestDatabaseProviderCachesUnderResolvedName covers the pool a caller naming
// fallbacks gets. Database\Migrate names "shop:migrate" before "shop" and
// the credential that answers is not the name it asked under. Caching the
// requested name would open another pool. On a private DSN such as an
// in-memory sqlite, that pool is a separate database. The applied schema
// would not be in the database the script queries.
func TestDatabaseProviderCachesUnderResolvedName(t *testing.T) {
	t.Parallel()
	provider := NewDatabaseProvider(Open)
	provider.Register("shop", "sqlite://:memory:")

	migrations, err := provider.Open(t.Context(), "shop:migrate", "shop")
	if err != nil {
		t.Fatalf("Open(shop:migrate, shop): %v", err)
	}

	queries, err := provider.Open(t.Context(), "shop")
	if err != nil {
		t.Fatalf("Open(shop): %v", err)
	}
	if queries != migrations {
		t.Errorf("Open(shop) returned %p, want the pool %p opened for the fallback", queries, migrations)
	}

	// A registered "shop:migrate" is the credential the migrations run with.
	// It opens a separate connection from the queries.
	provider.Register("shop:migrate", "sqlite://:memory:")
	privileged, err := provider.Open(t.Context(), "shop:migrate", "shop")
	if err != nil {
		t.Fatalf("Open(shop:migrate, shop): %v", err)
	}
	if privileged == queries {
		t.Error("Open(shop:migrate, shop) returned the pool of shop, want the one registered for shop:migrate")
	}
}

// TestNewIsolatesProviders is the property virtual hosting rests on: a
// provider resolves its configured connections alone. Two providers do not
// share the databases they open.
func TestNewIsolatesProviders(t *testing.T) {
	t.Parallel()
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
