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

// newRuntime builds the job runtime, calling register and the configured hooks after the standard library.
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

// cover installs a per-run collector and returns the function that adds its counts to the aggregator.
func (c config) cover(rt *runner.Runtime) func() {
	if c.coverage == nil {
		return func() {}
	}
	collector := coverage.New()
	rt.SetCoverage(collector)
	return func() { c.coverage.Add(collector) }
}
