package runner

import (
	"fmt"
)

// This file is the runtime's memory accounting: the walk over the live roots
// behind memory_get_usage(), the request baseline a host folds in, and the
// limit check the statement loop and the host-call burst guard run. The size
// estimation it walks with lives in memory.go.

// MemoryUsage returns request-scoped bytes, walking the live roots.
func (rt *Runtime) MemoryUsage() int64 {
	return rt.MemoryWalk()
}

// MemoryPeak returns the high-water mark, sampled at the walk points and not
// at every allocation the way php's is.
func (rt *Runtime) MemoryPeak() int64 {
	rt.MemoryWalk()
	return rt.memPeak
}

// MemoryLimit returns the configured memory limit.
func (rt *Runtime) MemoryLimit() Size {
	return rt.opts.MemoryLimit
}

// CompiledTree reports the programs, expressions and heap the tree costs.
func (rt *Runtime) CompiledTree() (programs, expressions int, heap int64) {
	return rt.includeCache.Len(), rt.exprCache.Len(), rt.includeCache.Precompiled()
}

// Precompiled reports whether this runtime precompiles.
func (rt *Runtime) Precompiled() bool { return rt.opts.Precompile }

// AccountRequest folds host-owned request values into the baseline the memory
// walk starts from, as an estimate and no audit.
func (rt *Runtime) AccountRequest(values ...any) {
	visited := make(visitedSet)
	for _, v := range values {
		rt.memBase += DeepSize(v, visited)
	}
}

// MemoryWalk recomputes live usage from the roots: the interpreter frame
// stack, globals, class statics, and any running flat VM's live values.
// A visited set keyed on container identity counts a value reachable through
// more than one variable once and terminates cycles. The result refreshes the
// cached usage and the peak.
func (rt *Runtime) MemoryWalk() int64 {
	visited := make(visitedSet)
	total := runtimeBaseline + rt.memBase
	for _, scope := range rt.frames {
		for name, val := range scope.vars {
			// Double-underscore slots are interpreter bookkeeping, not PHP
			// variables (the DefinedVars rule).
			if len(name) >= 2 && name[:2] == "__" {
				continue
			}
			total += 16 + int64(len(name)) + DeepSize(val, visited)
		}
	}
	for _, val := range rt.globals {
		total += DeepSize(val, visited)
	}
	rt.auto.Range(func(_ string, val any) {
		total += DeepSize(val, visited)
	})
	for _, bag := range rt.classStatics {
		for name, val := range bag {
			total += 16 + int64(len(name)) + DeepSize(val, visited)
		}
	}
	for _, bag := range rt.funcStatics {
		for name, val := range bag {
			total += 16 + int64(len(name)) + DeepSize(val, visited)
		}
	}
	for _, walk := range rt.vmWalkers {
		walk(func(v any) {
			total += DeepSize(v, visited)
		})
	}
	rt.memUsage = total
	rt.memPending = 0
	if total > rt.memPeak {
		rt.memPeak = total
	}
	return total
}

// lastMemoryUsage returns the cached result of the last walk. Trace spans
// record this once, with no walk apiece.
func (rt *Runtime) lastMemoryUsage() int64 {
	if rt.memUsage < runtimeBaseline {
		return runtimeBaseline
	}
	return rt.memUsage
}

// checkMemory walks and reports a RuntimeException when usage exceeds the
// configured MemoryLimit. Without a limit it does nothing.
func (rt *Runtime) checkMemory() error {
	if rt.opts.MemoryLimit <= 0 {
		return nil
	}
	if usage := rt.MemoryWalk(); rt.opts.MemoryLimit.Exceeds(usage) {
		return NewRuntimeException(fmt.Sprintf("Allowed memory size of %d bytes exhausted (%d bytes in use)", rt.opts.MemoryLimit.Bytes(), usage), 0)
	}
	return nil
}
