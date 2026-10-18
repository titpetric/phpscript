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
		// The runtime's own verdict and no reflect.Kind check: a PHP
		// closure is a value carrying its declaration, not a bare func, and
		// no other spelling of a callable is a func either.
		resolved, ok := rt.Callable(callback)
		if !ok {
			return fmt.Errorf("register_shutdown_function: argument must be callable")
		}
		// What the resolution answered, and not the argument it read: a
		// function name registers as a string, and the invoker that runs the
		// callbacks takes a func. Keeping the argument reports a finished
		// script as a failure, after its output, naming the type.
		rt.RegisterShutdown(resolved)
		return nil
	})
}
