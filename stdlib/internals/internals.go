// Package internals provides PHP internal memory and runtime introspection bindings.
package internals

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the internals bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs internals symbols into the runtime.
func Register(rt *runner.Runtime) {
	// The realUsage argument is accepted and ignored: the walk-based
	// estimate has no allocator/used distinction.
	rt.RegisterFunc("memory_get_usage", func(realUsage ...bool) int64 {
		return rt.MemoryUsage()
	})
	rt.RegisterFunc("memory_get_peak_usage", func(realUsage ...bool) int64 {
		return rt.MemoryPeak()
	})
}
