package ps

import (
	"context"
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/runner"
)

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	RegisterDefer(rt)
	RegisterSharedMemory(rt)
	RegisterShutdown(rt)
	RegisterSession(rt)
}

// RegisterSession installs session storage and manager bindings.
func RegisterSession(rt *runner.Runtime) {
	// Session\Storage\Memory is session storage backed by process memory; sessions vanish when the process exits.
	rt.RegisterConstructor("Session\\Storage\\Memory", NewSessionStorageMemory)
	// Session\Storage\Disk is session storage backed by files under $storage_path; with no path it uses the operating system's temporary directory.
	rt.RegisterConstructor("Session\\Storage\\Disk", NewSessionStorageDisk)
	// Session\Manager starts, reads and validates the request's session against the given $storage.
	rt.RegisterConstructor("Session\\Manager", NewSessionManager)
}

// RegisterDefer installs defer() in the global function namespace.
func RegisterDefer(rt *runner.Runtime) {
	// defer runs $callback when the enclosing function returns, or when the
	// script ends at top level; deferred callbacks run last-in, first-out.
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
	// register_shutdown_function runs $callback after the script finishes,
	// including after exit or an execution error, in registration order.
	rt.RegisterFunc("register_shutdown_function", func(callback any) error {
		value := reflect.ValueOf(callback)
		if !value.IsValid() || value.Kind() != reflect.Func || value.IsNil() {
			return fmt.Errorf("register_shutdown_function: argument must be callable")
		}
		rt.RegisterShutdown(callback)
		return nil
	})
}
