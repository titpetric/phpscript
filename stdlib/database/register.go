package database

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/titpetric/pdo/client"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// Register installs the Database and Database\Migrate bindings on rt.
func Register(rt *runner.Runtime) {
	// Database is the database client scripts query. $name selects a connection
	// registered with the host; the constructor throws when the name is not
	// registered or the pool cannot be opened.
	rt.RegisterConstructor("Database", func(ctx context.Context, name string) (*Database, error) {
		handle, err := provider(rt).Connect(ctx, name)
		if err != nil {
			return nil, err
		}

		database := &Database{Bridge: client.NewBridge(handle)}
		database.Bridge.WithObserver(database.observe)
		return database, nil
	})

	RegisterMigrate(rt)
	RegisterConnections(rt)
}

// RegisterConnections installs the statics that let a script see and extend
// the set of connections it can open.
//
// Connection names normally come from the environment the host was started
// with, which is right when the operator owns the list. An application that
// keeps its connections in a table owns the list itself, and has nowhere to
// put them: the provider is built once, before the first request, and putenv
// writes to the script environment, which the provider never reads.
func RegisterConnections(rt *runner.Runtime) {
	// Database::connections returns the names this script can open, sorted.
	rt.RegisterFunc("Database::connections", func() ([]string, error) {
		extended, ok := provider(rt).(model.ExtendedDatabaseProvider)
		if !ok {
			return nil, fmt.Errorf("Database::connections(): connections cannot be listed")
		}
		names := extended.List()
		sort.Strings(names)
		return names, nil
	})

	// Database::register adds the connection $name to the set new Database()
	// resolves against, with $dsn in "<driver>://<dsn>" form. Registering a
	// name that already means the same thing does nothing; registering it
	// with a different DSN closes the pool opened for the old one.
	rt.RegisterFunc("Database::register", func(name string, dsn string) (bool, error) {
		if name == "" {
			return false, fmt.Errorf("Database::register(): argument #1 ($name) must not be empty")
		}
		if dsn == "" {
			return false, fmt.Errorf("Database::register(): argument #2 ($dsn) must not be empty")
		}

		extended, ok := provider(rt).(model.ExtendedDatabaseProvider)
		if !ok {
			return false, fmt.Errorf("Database::register(): connections cannot be registered")
		}

		// A virtual host resolves through its own provider, so a site
		// registering a connection cannot reach another site's.
		extended.Register(strings.ToLower(name), dsn)
		return true, nil
	})
}

// RegisterMigrate installs the Database\Migrate binding.
func RegisterMigrate(rt *runner.Runtime) {
	// Database\Migrate loads a set of SQL migrations from the script filesystem
	// and runs them against the named connection.
	rt.RegisterConstructor("Database\\Migrate", func(ctx context.Context, names ...string) (*DatabaseMigrate, error) {
		database, err := provider(rt).Open(ctx, names...)
		if err != nil {
			return nil, err
		}
		return &DatabaseMigrate{
			database: database,
			root:     rt.FS(),
			workDir:  rt.WorkDir(),
		}, nil
	})
}

// provider returns the connections rt resolves through. A host that configured
// none, which is every CLI run, gets the process environment; a virtual host
// sets its own and is held to it.
func provider(rt *runner.Runtime) model.DatabaseProvider {
	if configured := rt.Database(); configured != nil {
		return configured
	}
	return Default
}
