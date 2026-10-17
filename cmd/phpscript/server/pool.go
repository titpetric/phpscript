package server

import (
	"io"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// buildRuntime constructs one runtime of this site's pool: the standard
// library, the site's filesystem root, its caches and its observers, installed
// once and serving every request the pool hands it to. FreezeStdlib marks the
// constants and the environment as the host's, so a reset between requests
// keeps them and drops what a script defined.
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
