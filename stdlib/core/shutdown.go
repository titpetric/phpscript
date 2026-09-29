package core

import (
	"fmt"

	"github.com/titpetric/phpscript/runner"
)

// init contributes register_shutdown_function() to stdlib.Register.
func init() {
	runner.RegisterBinding(RegisterShutdown)
}

// RegisterShutdown installs register_shutdown_function() in the global
// function namespace. Callbacks run in registration order when Runtime.Run
// finishes, including after exit or an execution error.
func RegisterShutdown(rt *runner.Runtime) {
	// register_shutdown_function runs $callback after the script finishes,
	// including after exit or an execution error, in registration order.
	rt.RegisterFunc("register_shutdown_function", func(callback any) error {
		// The runtime's own answer rather than a reflect.Kind check: a PHP
		// closure is a value carrying its declaration, not a bare func, and
		// every other spelling of a callable was never a func either.
		if _, ok := rt.Callable(callback); !ok {
			return fmt.Errorf("register_shutdown_function: argument must be callable")
		}
		rt.RegisterShutdown(callback)
		return nil
	})
}
