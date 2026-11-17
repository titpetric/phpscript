package server

import (
	"io"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// buildRuntime constructs one pooled runtime, with the standard library installed once.
func (h *handler) buildRuntime() *runner.Runtime {
	newRuntime := runner.New
	if h.flatstack {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(io.Discard, h.phpOptions)
	rt.SetIncludeCache(h.includeCache)
	rt.SetExprCache(h.exprCache)
	for _, observer := range h.observers {
		rt.Observe(observer)
	}
	stdlib.Register(rt)
	if h.rootDir != "" {
		stdlib.RegisterFS(rt, h.rootDir)
	}
	rt.FreezeStdlib()
	return rt
}
