package core

import (
	"fmt"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the Closure statics to stdlib.Register.
func init() {
	runner.RegisterArea(`Runtime\PS\Core\Closure`, registerClosure)
}

func registerClosure(rt *runner.Runtime) {
	// Closure::bind rebinds a closure's scope. phpscript enforces no property
	// visibility. A scope change has nothing to alter. The call answers the
	// closure unchanged. Rebinding `$this` would change the body's view. The
	// call refuses that with an error, and ignores nothing.
	rt.RegisterFunc("Closure::bind", func(closure any, args ...any) (any, error) {
		if len(args) > 0 && args[0] != nil {
			return nil, fmt.Errorf("Closure::bind(): rebinding $this is not supported")
		}
		if _, ok := rt.Callable(closure); !ok {
			return nil, fmt.Errorf("Closure::bind(): argument #1 ($closure) must be a closure")
		}
		return closure, nil
	})
	// Closure::fromCallable returns the closure for $callback; a value that
	// is not callable is an error.
	rt.RegisterFunc("Closure::fromCallable", func(callback any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, fmt.Errorf("Closure::fromCallable(): argument #1 ($callback) is not callable")
		}
		return fn, nil
	})
}
