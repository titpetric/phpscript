package database

import (
	"context"

	"github.com/titpetric/pdo/client"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// Register installs the Database and Database\Migrate bindings on rt.
func Register(rt *runner.Runtime) {
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
}

// RegisterMigrate installs the Database\Migrate binding.
func RegisterMigrate(rt *runner.Runtime) {
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
