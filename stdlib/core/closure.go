package core

import (
	"fmt"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the Closure statics to stdlib.Register.
func init() {
	runner.RegisterBinding(registerClosure)
}

func registerClosure(rt *runner.Runtime) {
	// Closure::bind rebinds a closure's scope. phpscript enforces no property
	// visibility, so a scope change has nothing to alter and the closure is
	// returned as it is. Rebinding `$this` would change what the body sees and
	// is therefore refused rather than silently ignored.
	rt.RegisterFunc("Closure::bind", func(closure any, args ...any) (any, error) {
		if len(args) > 0 && args[0] != nil {
			return nil, fmt.Errorf("Closure::bind(): rebinding $this is not supported")
		}
		if _, ok := rt.Callable(closure); !ok {
			return nil, fmt.Errorf("Closure::bind(): argument #1 ($closure) must be a closure")
		}
		return closure, nil
	})
	// Closure::fromCallable returns the closure for $callback; a value that is not callable is an error.
	rt.RegisterFunc("Closure::fromCallable", func(callback any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, fmt.Errorf("Closure::fromCallable(): argument #1 ($callback) is not callable")
		}
		return fn, nil
	})
}
