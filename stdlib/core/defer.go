package core

import (
	"context"
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/runner"
)

// init contributes defer() to stdlib.Register.
func init() {
	runner.RegisterBinding(RegisterDefer)
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
