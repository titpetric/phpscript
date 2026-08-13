package ps

import (
	"context"
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/status"
)

// Register installs the phpscript extensions into the runtime.
// These are all custom API's and not PHP standard library ones.
func Register(rt *runner.Runtime) {
	RegisterDatabase(rt)
	RegisterDefer(rt)
	RegisterSharedMemory(rt)
	RegisterShutdown(rt)

	rt.RegisterFunc("span", status.Span)
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
