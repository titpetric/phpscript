package tests

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sync"
	"testing/fstest"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/runner"
)

// ShareSources lays over the fixtures of one directory a filesystem holding
// every body in it and precompiling that tree compiles them all at once and
// not one per fixture.
func ShareSources(fixtures []*Fixture) {
	byRoot := map[string]fstest.MapFS{}
	for _, f := range fixtures {
		root := f.cacheRoot()
		if byRoot[root] == nil {
			byRoot[root] = fstest.MapFS{}
		}
		byRoot[root][f.sourceName()] = &fstest.MapFile{Data: []byte(f.PHP)}
	}
	for _, f := range fixtures {
		f.sources = byRoot[f.cacheRoot()]
	}
}

// Cache scopes, naming how far a parsed include and a compiled expression
// travel. The CLI spells them as --cache=off|worker.
const (
	CacheOff    = "off"
	CacheWorker = "worker"
)

// SetCacheScope says how far this fixture's parse caches reach.
func (f *Fixture) SetCacheScope(scope string) {
	f.cacheScope = scope
}

// caches returns this run's include and expression caches for the fixture's cache scope.
func (f *Fixture) caches(ctx context.Context) (*runner.IncludeCache, *runner.ExprCache) {
	switch f.cacheScope {
	case CacheOff:
		return runner.NewIncludeCache(), runner.NewExprCache()
	}
	w := currentWorker(ctx)
	// includeFor takes the fixture and not its trees, because this runs per
	// execution: building them means building the fixture's whole options
	// struct, and a method value handed over as a thunk allocates a closure
	// every time for a cache the first run fills.
	return w.includeFor(f.cacheRoot(), f, w.expr), w.expr
}

// precompileFS returns the fixture's root and its shared bodies as two filesystems.
func (f *Fixture) precompileFS() []fs.FS {
	trees := []fs.FS{f.runnerOptions().RootFS}
	if f.sources != nil {
		trees = append(trees, f.sources)
	}
	return trees
}

// flatCaches is caches for the bytecode runtime.
func (f *Fixture) flatCaches(ctx context.Context) (*flatstack.IncludeCache, *flatstack.ExprCache) {
	switch f.cacheScope {
	case CacheOff:
		return flatstack.NewIncludeCache(), flatstack.NewExprCache()
	}
	w := currentWorker(ctx)
	return w.flatIncludeFor(f.cacheRoot(), f), w.flatExpr
}

// cacheMode names the parse mode a fixture's caches hold: the two modes parse one body differently.
func (f *Fixture) cacheMode() string {
	if f.runnerOptions().GoRuntime() {
		return runner.RuntimeGo
	}
	return runner.RuntimePHPScript
}

// acquireRuntime returns the run's runtime and whether the runtime is fresh and still needs bindings.
func (f *Fixture) acquireRuntime(ctx context.Context, out io.Writer) (*runner.Runtime, bool) {
	include, expr := f.caches(ctx)

	build := func() *runner.Runtime {
		rt := runner.New(out, f.runnerOptions())
		rt.SetIncludeCache(include)
		rt.SetExprCache(expr)
		return rt
	}

	if f.cacheScope != CacheWorker {
		if f.interp != nil {
			// The same fixture again, under --count: the program and its AST
			// are the ones already compiled and the session resets and the
			// caches stay.
			f.interp.ResetSession(out, f.stdin())
			return f.interp, false
		}
		return build(), true
	}

	w := currentWorker(ctx)
	key := f.runtimeKey()
	if w.interp != nil && w.interpKey == key {
		// Reset, not ResetSession: the last fixture's compiled expressions and
		// source spans key on AST nodes this one will never look up. They
		// would hold that fixture's tree alive behind them.
		if f.interp == w.interp {
			w.interp.ResetSession(out, f.stdin())
		} else {
			w.interp.Reset(out, f.stdin())
		}
		return w.interp, false
	}

	rt := build()
	w.interp = rt
	w.interpKey = key
	return rt, true
}

// runtimeKey names what a runtime serves, and a worker rebuilds when the key changes.
func (f *Fixture) runtimeKey() string {
	if f.runtimeKeyS == "" {
		f.runtimeKeyS = fmt.Sprintf("%s\x00%s\x00%v", f.cacheRoot(), f.Runtime, f.Options)
	}
	return f.runtimeKeyS
}

// worker holds what one serial worker loop reuses across the fixtures it runs.
type worker struct {
	include     map[string]*runner.IncludeCache
	flatInclude map[string]*flatstack.IncludeCache
	expr        *runner.ExprCache
	flatExpr    *flatstack.ExprCache

	// The runtime this worker reuses, and the key it serves. One
	// runtime per worker, and not one per fixture, is what bounds what a
	// run holds by --parallel and not by the number of fixtures.
	interp    *runner.Runtime
	interpKey string
	flatRT    *flatstack.Runtime
	flatKey   string
}

func newWorker() *worker {
	return &worker{
		include:     map[string]*runner.IncludeCache{},
		flatInclude: map[string]*flatstack.IncludeCache{},
		expr:        runner.NewExprCache(),
		flatExpr:    flatstack.NewExprCache(),
	}
}

// includeFor returns this worker's cache for root, precompiling its tree on first use.
func (w *worker) includeFor(root string, f *Fixture, expr *runner.ExprCache) *runner.IncludeCache {
	key := root + "\x00" + f.cacheMode()
	if c, ok := w.include[key]; ok {
		return c
	}
	c := runner.NewIncludeCache()
	w.include[key] = c
	for _, tree := range f.precompileFS() {
		runner.Precompiler{Root: tree, Includes: c, Exprs: expr, GoRuntime: f.runnerOptions().GoRuntime()}.Run()
	}
	return c
}

// flatIncludeFor is includeFor for the bytecode runtime.
func (w *worker) flatIncludeFor(root string, f *Fixture) *flatstack.IncludeCache {
	key := root + "\x00" + f.cacheMode()
	if c, ok := w.flatInclude[key]; ok {
		return c
	}
	c := flatstack.NewIncludeCache()
	w.flatInclude[key] = c
	return c
}

// workers is the pool a worker-scoped run draws from, indexed by WithWorker's id, zero outside a loop.
var (
	workersMu sync.Mutex
	workers   = map[int]*worker{}
	workerKey = new(int)
)

// currentWorker returns the worker named by ctx, which holds the worker id set by the command.
func currentWorker(ctx context.Context) *worker {
	id := 0
	if got, ok := ctx.Value(workerKey).(int); ok {
		id = got
	}
	workersMu.Lock()
	defer workersMu.Unlock()
	if w, ok := workers[id]; ok {
		return w
	}
	w := newWorker()
	workers[id] = w
	return w
}

// cacheRoot returns the absolute tree a cached include path is relative to.
func (f *Fixture) cacheRoot() string {
	if f.rootKey != "" {
		return f.rootKey
	}
	f.rootKey = f.buildCacheRoot()
	return f.rootKey
}

// buildCacheRoot spells the cache key from the root and any app root.
func (f *Fixture) buildCacheRoot() string {
	root := absPath(f.RootDir())
	if f.appRoot != "" {
		return root + "\x00" + absPath(f.appRoot)
	}
	return root
}

// absPath returns the absolute spelling, or p itself when the working directory is unreadable.
func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// cleanState reports whether the run drops what it built when it ends.
func (f *Fixture) cleanState() bool { return f.cacheScope == CacheOff }

// acquireFlatRuntime is acquireRuntime for the bytecode runtime, returning the worker's flat runtime.
func (f *Fixture) acquireFlatRuntime(ctx context.Context, out io.Writer) (*flatstack.Runtime, *flatstack.ExprCache, bool) {
	include, expr := f.flatCaches(ctx)

	build := func() *flatstack.Runtime {
		rt := flatstack.New(out, f.runnerOptions())
		rt.SetIncludeCache(include)
		rt.SetExprCache(expr)
		return rt
	}

	if f.cacheScope != CacheWorker {
		if f.flatRT != nil {
			// The same fixture again, under --count: the session resets and
			// the caches stay.
			f.flatRT.ResetSession(out, f.stdin())
			return f.flatRT, f.flatExpr, false
		}
		return build(), expr, true
	}

	w := currentWorker(ctx)
	key := f.runtimeKey()
	if w.flatRT != nil && w.flatKey == key {
		// Reset, not ResetSession, between fixtures: see acquireRuntime.
		if f.flatRT == w.flatRT {
			w.flatRT.ResetSession(out, f.stdin())
		} else {
			w.flatRT.Reset(out, f.stdin())
		}
		return w.flatRT, expr, false
	}

	rt := build()
	w.flatRT = rt
	w.flatKey = key
	return rt, expr, true
}
