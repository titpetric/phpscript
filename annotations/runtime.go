package annotations

import (
	"context"
	"io"
	"io/fs"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// RuntimeFunc customizes a PHP runtime before an annotated PHP file executes.
type RuntimeFunc func(*runner.Runtime)

// newRuntime builds the runtime one annotated PHP file executes in. The sapi
// name separates a routed endpoint ("http") from a startup job ("cli").
//
// The register hooks run after the standard library is installed and before the
// caller's RuntimeFunc options, so an option can always wrap what came before
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
