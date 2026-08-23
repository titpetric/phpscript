package core

import (
	"fmt"
	"reflect"

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
		value := reflect.ValueOf(callback)
		if !value.IsValid() || value.Kind() != reflect.Func || value.IsNil() {
			return fmt.Errorf("register_shutdown_function: argument must be callable")
		}
		rt.RegisterShutdown(callback)
		return nil
	})
}
