package database

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

// TestDatabaseMigrate covers the handle a script holds on a schema: the
// connection it runs the SQL on, the filesystem it reads the files off, and the
// project mig records applied files under. Running the same set twice applies
// each file once, because mig reads its own record to decide, so a deployment
// that migrates on every process start does not replay the schema.
func TestDatabaseMigrate(t *testing.T) {
	t.Parallel()
	migrations := newLiveMigrate(t, fstest.MapFS{
		"schema/0001-users.up.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY);"),
		},
	})
	if err := migrations.Load("schema/*.up.sql"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, pass := range []string{"first", "second"} {
		if err := migrations.Run(t.Context()); err != nil {
			t.Fatalf("Run, %s pass: %v", pass, err)
		}
	}

	var project string
	if err := migrations.database.GetContext(t.Context(), &project,
		"SELECT DISTINCT project FROM migrations"); err != nil {
		t.Fatalf("reading the migration record: %v", err)
	}
	if project != "shop" {
		t.Errorf("recorded project = %q, want the connection name %q", project, "shop")
	}
}

// TestDatabaseMigrate_Load covers the glob a script names its migrations with.
// A script writes the pattern relative to itself, so the handle joins in the
// runtime work directory and takes the leading "./" off; what the pattern
// matched is what mig records, so the spelling has to settle to one form.
func TestDatabaseMigrate_Load(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		workDir string
		pattern string
		want    string
	}{
		{name: "relative", pattern: "./schema/*.up.sql", want: "schema/*.up.sql"},
		{name: "bare", pattern: "schema/*.up.sql", want: "schema/*.up.sql"},
		{name: "rooted", pattern: "/schema/*.up.sql", want: "schema/*.up.sql"},
		{name: "dot work directory", workDir: ".", pattern: "./schema/*.up.sql", want: "schema/*.up.sql"},
		{name: "under a site", workDir: "sites/shop", pattern: "./schema/*.up.sql", want: "sites/shop/schema/*.up.sql"},
	} {
		t.Run(test.name, func(t *testing.T) {
			migrations := &DatabaseMigrate{workDir: test.workDir}
			if err := migrations.Load(test.pattern); err != nil {
				t.Fatalf("Load(%q): %v", test.pattern, err)
			}
			if migrations.pattern != test.want {
				t.Errorf("pattern = %q, want %q", migrations.pattern, test.want)
			}
		})
	}
}

// TestDatabaseMigrate_Run covers applying a schema, and the empty set. Every
// statement of every matched file runs against the connection, so the tables
// are there when Run returns. A pattern matching nothing returns nil: a script
// can hold a schema directory it has not filled yet, and the binding has
// always let such a script start.
func TestDatabaseMigrate_Run(t *testing.T) {
	t.Parallel()
	root := fstest.MapFS{
		"schema/0001-users.up.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY);\nCREATE TABLE sessions (id TEXT);"),
		},
	}

	migrations := newLiveMigrate(t, root)
	if err := migrations.Load("schema/*.up.sql"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := migrations.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var tables []string
	if err := migrations.database.SelectContext(t.Context(), &tables,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name IN ('users', 'sessions') ORDER BY name"); err != nil {
		t.Fatalf("reading the schema back: %v", err)
	}
	if len(tables) != 2 || tables[0] != "sessions" || tables[1] != "users" {
		t.Errorf("tables = %v, want sessions and users", tables)
	}

	empty := newLiveMigrate(t, root)
	if err := empty.Load("unwritten/*.up.sql"); err != nil {
		t.Fatalf("Load over an empty directory: %v", err)
	}
	if err := empty.Run(t.Context()); err != nil {
		t.Errorf("Run over an empty set = %v, want nil", err)
	}
}

// newLiveMigrate returns a migration handle over an in-memory sqlite and the
// given script filesystem. It holds the pool to one connection, because a
// second connection to ":memory:" opens a second database and would find no
// schema. The blank import in provider_test.go registers the driver, and it
// compiles into this same test binary.
func newLiveMigrate(t *testing.T, root fs.FS) *DatabaseMigrate {
	t.Helper()

	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	return &DatabaseMigrate{database: db, root: root, project: "shop"}
}
