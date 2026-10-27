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

// caches answers the include and expression caches this run installs.
//
// Off builds a pair for this run alone. Worker takes the pair belonging to the
// goroutine running it and the fixtures one worker walks share what they parse
// and two workers share nothing. Shared is the process-wide pair, keyed by
// include root because a cache keys on the path a script wrote and two
// folders can both hold a code/functions.php.
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

// precompileFS is the tree the precompiler walks: the fixture's own root, and
// the bodies of the directory as a second filesystem.
//
// They are two and not one because a walk cannot see through the overlay.
// unionFS answers Open from the first layer that has the name and implements
// no ReadDir and fs.WalkDir over it lists the real tree alone and skips
// every body. Merging the listings instead is not open: scandir(".") is
// the same call, and a fixture reading its own directory would start seeing
// the other fixtures' bodies in it.
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
	return w.flatIncludeFor(f.cacheRoot()), w.flatExpr
}

// acquireRuntime answers the runtime this run executes on, and whether that
// runtime is fresh and still needs its bindings.
//
// Worker scope returns the worker's own runtime, Reset for this run and never
// rebuilt: a reset runtime keeps the maps and buckets it grew and the next
// fixture allocates nothing to start, and --parallel bounds what a run holds
// and not the number of fixtures. The worker rebuilds it only when the
// fixture answers a different runtimeKey, which is once per folder and not
// once per fixture.
//
// Off scope builds one per run and lets it go when the run ends, which is the
// clean state that mode is for.
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
		// source spans key on AST nodes this one will never look up, and
		// they would hold that fixture's tree alive behind them.
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

// runtimeKey names what a runtime serves. A worker reuses its runtime
// when the next fixture answers the same key and builds another when it does
// not: runner.New takes RootFS at construction and stdlib.RegisterFS binds the
// file bindings to a directory and neither survives a move. Fixtures arrive
// grouped by folder and the key changes once per group and not once per
// fixture.
func (f *Fixture) runtimeKey() string {
	if f.runtimeKeyS == "" {
		f.runtimeKeyS = fmt.Sprintf("%s\x00%v", f.cacheRoot(), f.Options)
	}
	return f.runtimeKeyS
}

// worker holds what one serial worker loop reuses across the fixtures it runs.
//
// includeFor keys the include caches by include root and never pools them into
// one. A cache keys on the path a script wrote and one cache spanning two roots
// would serve a fixture in one folder the code/functions.php belonging to
// another - and with --autoload, autoload.php names a different file under
// every invocation root. The expression caches key on source text, which
// carries no root and one of each per worker is right.
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

// includeFor answers this worker's cache for one include root, precompiling the
// tree the first time a fixture of that root reaches it. A worker runs its
// fixtures serially and this takes no lock.
//
// The pass puts the fixtures in the cache before one of them runs: Precompiler
// parses and compiles every .php below the root, the bodies among them, once,
// and a fixture and a repeat of it then read the program back without
// building it again.
func (w *worker) includeFor(root string, f *Fixture, expr *runner.ExprCache) *runner.IncludeCache {
	if c, ok := w.include[root]; ok {
		return c
	}
	c := runner.NewIncludeCache()
	w.include[root] = c
	for _, tree := range f.precompileFS() {
		runner.Precompiler{Root: tree, Includes: c, Exprs: expr}.Run()
	}
	return c
}

// flatIncludeFor is includeFor for the bytecode runtime.
func (w *worker) flatIncludeFor(root string) *flatstack.IncludeCache {
	if c, ok := w.flatInclude[root]; ok {
		return c
	}
	c := flatstack.NewIncludeCache()
	w.flatInclude[root] = c
	return c
}

// workers is the pool a worker-scoped run draws from, indexed by the id
// WithWorker assigned. A run outside a worker loop - a single-threaded run, a
// Go test calling RunFixture directly - gets index zero, which is a worker like
// any other.
var (
	workersMu sync.Mutex
	workers   = map[int]*worker{}
	workerKey = new(int)
)

// currentWorker answers the caches for the worker ctx names. Go has no
// goroutine-local storage and the id rides the run's own context, already
// threaded from the command down to here.
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

// cacheRoot names the tree a cached include path is relative to.
//
// Absolute, because the relative spelling is ambiguous: a cached path is
// relative to a root, and two roots reached from different working directories
// can both carry the name "suite". A fixture given an app root resolves against
// that too and the key names both trees.
func (f *Fixture) cacheRoot() string {
	if f.rootKey != "" {
		return f.rootKey
	}
	f.rootKey = f.buildCacheRoot()
	return f.rootKey
}

// buildCacheRoot spells the key. cacheRoot holds the result because absPath
// reaches the OS for the working directory, and each execution needs it once.
func (f *Fixture) buildCacheRoot() string {
	root := absPath(f.RootDir())
	if f.appRoot != "" {
		return root + "\x00" + absPath(f.appRoot)
	}
	return root
}

// absPath answers the absolute spelling, or the given one when it cannot read
// the working directory - a key that is merely ambiguous beats no key.
func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// cleanState reports whether the run drops what it built when it ends.
func (f *Fixture) cleanState() bool { return f.cacheScope == CacheOff }

// acquireFlatRuntime is acquireRuntime for the bytecode runtime: worker scope
// returns the worker's flat runtime, Reset for this run and rebuilt once
// per folder and not once per fixture. Building a runtime is a full
// stdlib registration, which a single-shot run pays per fixture on this
// engine while the interpreter amortises it per folder.
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
