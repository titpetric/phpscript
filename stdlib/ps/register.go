package ps

import (
	"context"
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/runner"
)

// Register installs the phpscript extensions into the runtime.
func Register(rt *runner.Runtime) {
	RegisterDatabase(rt)
	RegisterDefer(rt)
	RegisterSHM(rt)
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
