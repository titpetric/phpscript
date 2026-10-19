// Package internals carries PHP's internal memory and runtime introspection bindings.
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
	// memory_get_usage reports the live memory estimate of the running script
	// in bytes; it accepts $real_usage and ignores it, as the walk-based
	// estimate has no allocator/used distinction.
	rt.RegisterFunc("memory_get_usage", func(realUsage ...bool) int64 {
		return rt.MemoryUsage()
	})
	// memory_get_peak_usage reports the peak memory estimate observed for the
	// running script in bytes; it accepts $real_usage and ignores it.
	rt.RegisterFunc("memory_get_peak_usage", func(realUsage ...bool) int64 {
		return rt.MemoryPeak()
	})
}
