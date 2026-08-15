package ps

import (
	"context"
	"fmt"
	"reflect"

	"github.com/titpetric/pdo/client"
	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/runner"
)

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	RegisterDefer(rt)
	RegisterSharedMemory(rt)
	RegisterShutdown(rt)
	RegisterSession(rt)
	RegisterDatabaseMigrate(rt)

	rt.RegisterConstructor("Database", func(ctx context.Context, name string) (*Database, error) {
		handle, err := platform.Database.Connect(ctx, name)
		if err != nil {
			return nil, err
		}

		database := &Database{Bridge: client.NewBridge(handle)}
		database.Bridge.WithObserver(database.observe)
		return database, nil
	})
}

// RegisterDatabaseMigrate installs the Database\Migrate binding.
func RegisterDatabaseMigrate(rt *runner.Runtime) {
	rt.RegisterConstructor("Database\\Migrate", func(ctx context.Context, names ...string) (*DatabaseMigrate, error) {
		database, err := platform.Database.Open(ctx, names...)
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

// RegisterSession installs session storage and manager bindings.
func RegisterSession(rt *runner.Runtime) {
	rt.RegisterConstructor("Session\\Storage\\Memory", NewSessionStorageMemory)
	rt.RegisterConstructor("Session\\Storage\\Disk", NewSessionStorageDisk)
	rt.RegisterConstructor("Session\\Manager", NewSessionManager)
}

// RegisterDefer installs defer() in the global function namespace.
func RegisterDefer(rt *runner.Runtime) {
	rt.RegisterFunc("defer", func(ctx context.Context, callback any) error {
		value := reflect.ValueOf(callback)
		if !value.IsValid() || value.Kind() != reflect.Func || value.IsNil() {
			return fmt.Errorf("defer: argument must be callable")
		}
		scope, ok := runner.ScopeFromContext(ctx)
		if !ok {
			return fmt.Errorf("defer: no active execution frame")
		}
		scope.Defer(callback)
		return nil
	})
}

// RegisterShutdown installs register_shutdown_function() in the global
// function namespace. Callbacks run in registration order when Runtime.Run
// finishes, including after exit or an execution error.
func RegisterShutdown(rt *runner.Runtime) {
	rt.RegisterFunc("register_shutdown_function", func(callback any) error {
		value := reflect.ValueOf(callback)
		if !value.IsValid() || value.Kind() != reflect.Func || value.IsNil() {
			return fmt.Errorf("register_shutdown_function: argument must be callable")
		}
		rt.RegisterShutdown(callback)
		return nil
	})
}
