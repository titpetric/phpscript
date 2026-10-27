package annotations

import (
	"context"
	"io"
	"io/fs"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/stdlib"
)

// RuntimeFunc customizes a PHP runtime before an annotated PHP file executes.
type RuntimeFunc func(*runner.Runtime)

// newRuntime builds the runtime one annotated startup or scheduled job
// executes in, under the "cli" sapi. A routed endpoint takes a pooled runtime
// from its handler instead; see handler.buildRuntime.
//
// The register hooks run once the standard library sits on the runtime, before the
// caller's RuntimeFunc options and an option can always wrap what came before
// it, a runtime context value in particular.
func (c config) newRuntime(ctx context.Context, root fs.FS, out io.Writer, sapi string, register ...RuntimeFunc) *runner.Runtime {
	options := c.runnerOptions
	options.RootFS = root
	options.SAPI = sapi

	construct := runner.New
	if c.flatstack {
		construct = runner.NewFlatStack
	}
	rt := construct(out, options)
	rt.SetContext(ctx)
	for _, observer := range c.observers {
		rt.Observe(observer)
	}

	stdlib.Register(rt)
	if c.rootDir != "" {
		stdlib.RegisterFS(rt, c.rootDir)
	}
	for _, fn := range register {
		fn(rt)
	}
	for _, fn := range c.runtimeFuncs {
		fn(rt)
	}
	return rt
}

// cover installs a collector on rt and returns the function that folds what it
// counted into the process aggregator. With no aggregator both do nothing.
//
// The collector is per run because it keys statements by the AST node the
// parser produced: one kept across runs would grow with every re-parse, while
// the aggregator holds one entry per statement range at any number of hits.
func (c config) cover(rt *runner.Runtime) func() {
	if c.coverage == nil {
		return func() {}
	}
	collector := coverage.New()
	rt.SetCoverage(collector)
	return func() { c.coverage.Add(collector) }
}
