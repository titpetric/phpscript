package tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// programLoader is the half of a runtime the harness loads through, which both
// engines answer.
type programLoader interface {
	LoadFile(string) (*model.Program, error)
}

// load answers the fixture's program, reading its body through the runtime the
// first time so the parse and the compile land in the caches a precompile pass
// fills. It holds the program because a repeat uses the same one: --count
// re-runs the body, it does not re-read it.
func (f *Fixture) load(rt programLoader) (*model.Program, error) {
	if f.prog != nil {
		return f.prog, nil
	}
	prog, err := rt.LoadFile(f.sourceName())
	if err != nil {
		return nil, err
	}
	f.prog = prog
	return prog, nil
}

func (f *Fixture) stdin() io.Reader {
	content := f.Request.Stdin
	if content == "" {
		content = f.Stdin
	}
	if content == "" {
		return strings.NewReader("")
	}
	return strings.NewReader(content)
}

func executeFixturePHP(ctx context.Context, f *Fixture) (string, runner.Context, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cleanState() {
		// A clean state is a new runtime, not only an empty cache. A runtime
		// keeps every declaration it hoisted out of an include, and this
		// fixture holds the runtime for the whole run and reusing one carries
		// the last fixture's functions and classes into the next and keeps
		// their ASTs alive besides.
		defer func() { f.interp = nil }()
	}

	var out strings.Builder
	reqCtx := buildFixtureRequestContext(f)
	rt, built := f.acquireRuntime(ctx, &out)
	if built {
		rt.RegisterConstructor("Storage", NewStorage)
		rt.RegisterConstructor("FailStorage", NewFailStorage)
		registerPanicBindings(rt)
		// Register installs the shims rooted at the process working directory;
		// RegisterFS rebinds them to the fixture and it has to run after it.
		stdlib.Register(rt)
		stdlib.RegisterFS(rt, f.RootDir())
		rt.FreezeStdlib()
	}
	// The fixture names itself before the load, because a runtime keeps the
	// first name it takes: a fixture reading __DIR__ reads its own directory,
	// never the file sourceFS serves its body from. An empty __DIR__
	// silently turns __DIR__ . "/x" into "/x", which is then rejected as
	// escaping the root: a confusing two-step failure a long way from its cause.
	rt.UpdateFilename(f.Path)

	prog, err := f.load(rt)
	if err != nil {
		return "Internal Server Error", runner.Context{}, err
	}
	f.interp = rt

	f.interp.SetCoverage(f.coverage)
	f.interp.SetContext(ctx)
	reqCtx.Register(f.interp)

	if err := f.includePrelude(f.interp); err != nil {
		return "Internal Server Error", reqCtx, err
	}

	if err := f.interp.Run(prog); err != nil {
		if _, ok := runner.IsExit(err); ok {
			return out.String(), reqCtx, nil
		}
		return "Internal Server Error", reqCtx, err
	}
	return out.String(), reqCtx, nil
}

// executeFlatstack mirrors executeFixturePHP on the flat bytecode runtime, and
// both in-process runners show the same host behavior: an uncaught error
// produces the host error body, and reqCtx.Register installs the request.
func executeFlatstack(ctx context.Context, f *Fixture) (string, runner.Context, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cleanState() {
		defer func() { f.flatRT = nil }()
	}

	var out strings.Builder
	reqCtx := buildFixtureRequestContext(f)
	rt, expr, built := f.acquireFlatRuntime(ctx, &out)
	if built {
		rt.RegisterConstructor("Storage", NewStorage)
		rt.RegisterConstructor("FailStorage", NewFailStorage)
		registerPanicBindings(rt)
		stdlib.Register(rt)
		stdlib.RegisterFS(rt, f.RootDir())
		rt.FreezeStdlib()
	}
	rt.UpdateFilename(f.Path)

	prog, err := f.load(rt)
	if err != nil {
		return "Internal Server Error", runner.Context{}, err
	}
	f.flatRT = rt
	f.flatExpr = expr

	// The gate the benchmarks hold to: the harness skips a program the
	// compiler rejects and never delegates it. The compile resolves through
	// the run's own expression cache and the one the gate pays is the one the
	// run reads back; the fixture caches the verdict and it pays once. A
	// violated interface contract is not a coverage gap: the flat path raises
	// it as the program's own error and the fixture runs and asserts it.
	if !f.flatChecked {
		f.flatChecked = true
		if err := f.flatExpr.EnsureFlat(prog); err != nil {
			var contract *model.InterfaceContractError
			if !errors.As(err, &contract) {
				f.flatErr = err
			}
		}
	}
	if f.flatErr != nil {
		return "", runner.Context{}, fmt.Errorf("%w: %v", ErrFlatstackUnsupported, f.flatErr)
	}
	f.flatRT.SetContext(ctx)
	reqCtx.Register(f.flatRT)

	if err := f.includePrelude(f.flatRT); err != nil {
		return "Internal Server Error", reqCtx, err
	}

	if err := f.flatRT.Run(prog); err != nil {
		if _, ok := flatstack.IsExit(err); ok {
			return out.String(), reqCtx, nil
		}
		return "Internal Server Error", reqCtx, err
	}
	return out.String(), reqCtx, nil
}

func buildFixtureRequestContext(f *Fixture) runner.Context {
	// NewContext already carries empty maps; only a fixture that declares a
	// section replaces one. The unconditional overwrite-then-refill spelled
	// five discarded map allocations per fixture run.
	reqCtx := runner.NewContext()
	if f.Request.Get != nil {
		reqCtx.Get = f.Request.Get
	}
	if f.Request.Post != nil {
		reqCtx.Post = f.Request.Post
	}
	if f.Request.Cookie != nil {
		reqCtx.Cookie = f.Request.Cookie
	}
	if f.Request.Env != nil {
		reqCtx.Env = f.Request.Env
	}
	if f.Request.Headers != nil {
		reqCtx.Headers = f.Request.Headers
	}

	// Populate standard $_SERVER environment
	reqCtx.Server["REQUEST_METHOD"] = "GET"
	reqCtx.Server["REQUEST_URI"] = "/"
	reqCtx.Server["QUERY_STRING"] = ""
	reqCtx.Server["HTTP_HOST"] = "localhost"
	reqCtx.Server["SERVER_PROTOCOL"] = "HTTP/1.1"

	for k, v := range reqCtx.Headers {
		key := "HTTP_" + strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
		reqCtx.Server[key] = v
	}

	// Parse args
	if f.Request.Args != nil {
		reqCtx.Argv = parseArgs(f.Request.Args, f.Path)
	}

	// The raw request body, which php://input answers with.
	if f.Request.Body != "" {
		reqCtx.SetRawBody([]byte(f.Request.Body))
	}

	return reqCtx
}
