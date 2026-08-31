package tests

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/stdlib"
)

//go:embed all:fixtures
var fixturesFS embed.FS

var phpFS fs.FS

var phpFSOnce sync.Once

func testPHPFS() fs.FS {
	phpFSOnce.Do(func() {
		var err error
		phpFS, err = fs.Sub(fixturesFS, "fixtures")
		if err != nil {
			panic(err)
		}
	})
	return phpFS
}

// fixtureArea is the fixtures of one area directory, in discovery order.
type fixtureArea struct {
	Name     string
	Fixtures []*Fixture
}

// embeddedFixtures walks the embedded tree and groups every .phpt by the area
// directory holding it. A fixture's include root is that directory, which is
// also where the php runner executes, so all three runners resolve a relative
// include to the same file.
func embeddedFixtures() ([]fixtureArea, error) {
	index := map[string]int{}
	var areas []fixtureArea

	err := fs.WalkDir(fixturesFS, "fixtures", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".phpt") {
			return nil
		}

		data, err := fixturesFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		fx, err := ParseFixture(data, p)
		if err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}

		dir := path.Dir(p)
		root, err := fs.Sub(fixturesFS, dir)
		if err != nil {
			return fmt.Errorf("sub %s: %w", dir, err)
		}
		fx.SetRootFS(root)

		name := path.Base(dir)
		if _, ok := index[name]; !ok {
			index[name] = len(areas)
			areas = append(areas, fixtureArea{Name: name})
		}
		at := index[name]
		areas[at].Fixtures = append(areas[at].Fixtures, fx)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return areas, nil
}

func errorChainContains(err error, substr string) bool {
	for err != nil {
		if strings.Contains(err.Error(), substr) {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// FixtureRequest contains request data exposed to PHP as superglobals.
type FixtureRequest struct {
	Args    any               `yaml:"args"` // map[string]string or []string or scalar
	Get     map[string]string `yaml:"get"`
	Post    map[string]string `yaml:"post"`
	Cookie  map[string]string `yaml:"cookie"`
	Env     map[string]string `yaml:"env"`
	Headers map[string]string `yaml:"headers"`
	Stdin   string            `yaml:"stdin"`
	Body    string            `yaml:"body"`
}

// FixtureResponse contains expected response assertions.
type FixtureResponse struct {
	Headers map[string]string `yaml:"headers"`
}

// Fixture is the parsed representation of a .phpt test file.
type Fixture struct {
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Error       string          `yaml:"error"`   // optional: expected uncaught error substring
	Stdin       string          `yaml:"stdin"`   // optional: top-level stdin contents
	Options     runner.Options  `yaml:"options"` // optional: runtime options for both engines (memory_limit, ...)
	Root        string          `yaml:"root"`    // optional: include root, relative to the fixture's own directory
	Serial      bool            `yaml:"serial"`  // optional: do not overlap this fixture with peers in its area
	Runner      FixtureRunners  `yaml:"runner"`  // optional: runners the fixture opts out of
	Request     FixtureRequest  `yaml:"request"`
	Response    FixtureResponse `yaml:"response"`

	PHP      string `yaml:"-"`
	Expected string `yaml:"-"`
	Path     string `yaml:"-"`

	appRoot    string
	cacheScope string
	includes   []string
	coverage   *coverage.Collector
	rootFS     fs.FS
	mu         sync.Mutex
	parsed     *model.Program
	parsedErr  error
	interp     *runner.Runtime
	flatRT     *flatstack.Runtime
}

// TestResult carries execution outcome for a single fixture.
type TestResult struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Path          string `json:"path"`
	Passed        bool   `json:"passed"`
	DurationMs    int64  `json:"duration_ms"`
	FailureReason string `json:"failure_reason,omitempty"`
	GotOutput     string `json:"got_output,omitempty"`
	WantOutput    string `json:"want_output,omitempty"`
	Error         string `json:"error,omitempty"`
	Runner        Runner `json:"runner,omitempty"`
	Skipped       bool   `json:"skipped,omitempty"`
}

// SetRootFS binds the filesystem a fixture's includes resolve against. The
// directory holding the .phpt is the right answer for every caller: it is what
// the php runner uses as its working directory, so all three runners resolve a
// relative include to the same file.
func (f *Fixture) SetRootFS(root fs.FS) {
	f.rootFS = root
}

// SetAppRoot points the fixture at the application root the test command ran
// in. The autoload folder convention and the prelude includes resolve there,
// while the fixture's own relative includes keep resolving against its
// directory: runnerOptions layers the two, fixture directory first.
//
// An include that is not there is not an error; the flag says what to load
// when the application provides it, and a tree without an autoload.php simply
// runs without one.
// Cache scopes, naming how far a parsed include and a compiled expression
// travel. The CLI spells them as --cache=off|worker|shared.
const (
	CacheOff    = "off"
	CacheWorker = "worker"
)

// SetCacheScope says how far this fixture's parse caches reach.
//
// The caches are what a suite trades memory for speed with. Worker, the
// default, gives each worker loop one set, reused by the fixtures that worker
// runs serially, so what is held scales with --parallel rather than with the
// number of fixtures. Off gives a run its own and drops them - and its runtime
// - when it ends, so a run is charged its own parsing and holds nothing
// afterwards.
func (f *Fixture) SetCacheScope(scope string) {
	f.cacheScope = scope
}

// caches answers the include and expression caches this run installs.
//
// Off builds a pair for this run alone. Worker takes the pair belonging to the
// goroutine running it, so the fixtures one worker walks share what they parse
// and two workers share nothing. Shared is the process-wide pair, keyed by
// include root because a cache is keyed by the path a script wrote and two
// folders can both hold a code/functions.php.
func (f *Fixture) caches(ctx context.Context) (*runner.IncludeCache, *runner.ExprCache) {
	switch f.cacheScope {
	case CacheOff:
		return runner.NewIncludeCache(), runner.NewExprCache()
	}
	w := currentWorker(ctx)
	return w.includeFor(f.cacheRoot()), w.expr
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

// acquireRuntime answers the runtime this run executes on, and whether it was
// just built and still needs its bindings.
//
// Worker scope hands back the worker's own runtime, Reset for this run rather
// than rebuilt: a reset runtime keeps the maps and buckets it grew, so the next
// fixture allocates nothing to start, and what a run holds is bounded by
// --parallel instead of by the number of fixtures. It is rebuilt only when the
// fixture answers a different runtimeKey, which is once per folder rather than
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
			// are the ones already compiled, so the session resets and the
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
		// source spans are keyed by AST nodes this one will never look up, and
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

// runtimeKey names what a runtime was built for. A worker reuses its runtime
// when the next fixture answers the same key and builds another when it does
// not: RootFS is fixed at construction and stdlib.RegisterFS binds the file
// bindings to a directory, so neither survives a move. Fixtures arrive grouped
// by folder, so the key changes once per group rather than once per fixture.
func (f *Fixture) runtimeKey() string {
	return fmt.Sprintf("%s\x00%v", f.cacheRoot(), f.Options)
}

// worker holds what one serial worker loop reuses across the fixtures it runs.
//
// The include caches are keyed by include root, not pooled into one. A cache is
// keyed by the path a script wrote, so one cache spanning two roots would serve
// a fixture in one folder the code/functions.php belonging to another - and
// with --autoload, autoload.php names a different file under every invocation
// root. The expression caches are keyed by source text, which carries no root,
// so one of each per worker is right.
type worker struct {
	include     map[string]*runner.IncludeCache
	flatInclude map[string]*flatstack.IncludeCache
	expr        *runner.ExprCache
	flatExpr    *flatstack.ExprCache

	// The runtime this worker reuses, and the key it was built for. One
	// runtime per worker rather than one per fixture is what bounds what a
	// run holds by --parallel instead of by the number of fixtures.
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

// includeFor answers this worker's cache for one include root, building it on
// first use. A worker runs its fixtures serially, so no lock is needed here.
func (w *worker) includeFor(root string) *runner.IncludeCache {
	if c, ok := w.include[root]; ok {
		return c
	}
	c := runner.NewIncludeCache()
	w.include[root] = c
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

// WithWorker marks ctx as belonging to worker id, so a fixture run under it
// takes that worker's caches. The command's worker loop calls this once per
// goroutine; everything else inherits worker zero.
func WithWorker(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, workerKey, id)
}

// currentWorker answers the caches for the worker ctx names. Go has no
// goroutine-local storage, so the id rides the context the run was started
// with - which is already threaded from the command down to here.
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
// can both be called "suite". A fixture given an app root resolves against that
// too, so the key names both trees.
func (f *Fixture) cacheRoot() string {
	root := absPath(f.RootDir())
	if f.appRoot != "" {
		return root + "\x00" + absPath(f.appRoot)
	}
	return root
}

// absPath answers the absolute spelling, or the given one when the working
// directory cannot be read - a key that is merely ambiguous beats no key.
func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// cleanState reports whether the run drops what it built when it ends.
func (f *Fixture) cleanState() bool { return f.cacheScope == CacheOff }

func (f *Fixture) SetAppRoot(root, autoload string, includes ...string) {
	f.appRoot = root
	f.includes = includes
	if autoload != "" {
		f.Options.Autoload = autoload
	}
}

// SetCoverage installs a statement-coverage collector for the fixture's
// runtime runner. Only that runner reports coverage: flatstack carries no
// coverage support, and the php runner is another process.
func (f *Fixture) SetCoverage(c *coverage.Collector) {
	f.coverage = c
}

// Coverage returns the collector installed with SetCoverage, or nil.
func (f *Fixture) Coverage() *coverage.Collector {
	return f.coverage
}

// RootDir returns the directory a fixture's includes resolve against. That is
// the directory holding it, which is also where the php runner executes, unless
// the fixture named another one with `root:`.
func (f *Fixture) RootDir() string {
	dir := filepath.Dir(f.Path)
	if f.Root == "" {
		return dir
	}
	return filepath.Join(dir, f.Root)
}

// realRoot reports whether the fixture asked for a directory on disk rather
// than the embedded tree. Such a fixture gets its own include caches: those are
// keyed by the path as the script wrote it, so a fixture reaching a different
// tree must not be served a program cached for the embedded one.
func (f *Fixture) realRoot() bool {
	return f.Root != "" || f.appRoot != ""
}

// unionFS resolves a path against each layer in order, first hit wins. It is
// how a fixture keeps its own directory as the include root while the
// application root the test command ran in answers for everything the fixture
// directory does not hold. fs.Stat and fs.ReadFile both fall back to Open, so
// Open is the whole contract.
type unionFS []fs.FS

func (u unionFS) Open(name string) (fs.File, error) {
	var firstErr error
	for _, layer := range u {
		file, err := layer.Open(name)
		if err == nil {
			return file, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return nil, firstErr
}

// includePrelude includes the files the test command named with --include
// before the fixture body runs. It runs every session, because ResetSession
// clears user declarations. A file that is not there is skipped: the flag
// names what to load when the application provides it.
func (f *Fixture) includePrelude(rt *runner.Runtime) error {
	for _, name := range f.includes {
		clean := strings.TrimPrefix(path.Clean(filepath.ToSlash(name)), "/")
		if _, err := fs.Stat(rt.FS(), clean); err != nil {
			continue
		}
		if _, err := rt.IncludeFile(clean); err != nil {
			return err
		}
	}
	return nil
}

// runnerOptions returns the fixture's options: frontmatter with the
// harness-owned fields filled in.
func (f *Fixture) runnerOptions() runner.Options {
	options := f.Options
	// The php column executes the php cli binary, so the runtime column
	// speaks as the same SAPI: a fixture probing php_sapi_name() must read
	// the same answer from both.
	options.SAPI = "cli"
	options.RootFS = f.rootFS
	if f.realRoot() {
		// A fixture that names a root wants the real filesystem: it is reaching
		// for a tree phpscript does not embed, a vendor directory being the
		// motivating case.
		options.RootFS = os.DirFS(f.RootDir())
	}
	if f.appRoot != "" {
		// The fixture directory answers first, so a fixture's own relative
		// include keeps meaning what it always meant; the application root
		// answers second, for the prelude include, the files it pulls in, and
		// the autoload folder, all of which sit outside every fixture directory.
		options.RootFS = unionFS{os.DirFS(f.RootDir()), os.DirFS(f.appRoot)}
	}
	if options.RootFS == nil {
		options.RootFS = testPHPFS()
	}
	options.Stdin = f.stdin()
	return options
}

// ParseFixture splits a .phpt file into its three sections and parses the YAML metadata.
func ParseFixture(data []byte, path ...string) (*Fixture, error) {
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	parts := strings.SplitN(normalized, "\n---\n", 3)
	if len(parts) != 3 {
		return nil, errors.New("malformed .phpt: want <yaml>---<php>---<output>")
	}

	var f Fixture
	if err := yaml.Unmarshal([]byte(parts[0]), &f); err != nil {
		return nil, fmt.Errorf("yaml metadata unmarshal error: %w", err)
	}

	if f.Name == "" {
		return nil, errors.New("fixture metadata missing required 'name'")
	}
	if f.Description == "" {
		return nil, errors.New("fixture metadata missing required 'description'")
	}

	f.PHP = strings.TrimPrefix(parts[1], "\n")
	f.Expected = parts[2]
	if len(path) > 0 {
		f.Path = path[0]
	}

	return &f, nil
}

// FindFixtures locates all .phpt files in the specified paths (files or directories).
func FindFixtures(paths []string) ([]*Fixture, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	var fixtures []*Fixture
	seen := make(map[string]bool)

	for _, p := range paths {
		p = filepath.Clean(p)
		recursive := false
		if strings.HasSuffix(p, "/...") || strings.HasSuffix(p, string(filepath.Separator)+"...") || p == "..." {
			recursive = true
			p = strings.TrimSuffix(p, "...")
			p = strings.TrimSuffix(p, string(filepath.Separator))
			if p == "" {
				p = "."
			}
		}

		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}

		if !info.IsDir() {
			if strings.HasSuffix(p, ".phpt") && !seen[p] {
				seen[p] = true
				fx, err := loadFixtureFile(p)
				if err != nil {
					return nil, err
				}
				fixtures = append(fixtures, fx)
			}
			continue
		}

		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != p && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".phpt") && !seen[path] {
				seen[path] = true
				fx, err := loadFixtureFile(path)
				if err != nil {
					return err
				}
				fixtures = append(fixtures, fx)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", p, err)
		}
	}

	sort.Slice(fixtures, func(i, j int) bool {
		return fixtures[i].Path < fixtures[j].Path
	})

	return fixtures, nil
}

func loadFixtureFile(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	fx, err := ParseFixture(data, path)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	fx.SetRootFS(os.DirFS(filepath.Dir(path)))
	return fx, nil
}

// RunFixture executes a single fixture against the default runtime.
func RunFixture(ctx context.Context, f *Fixture) *TestResult {
	return RunFixtureOn(ctx, f, RunnerRuntime)
}

// RunFixtureOn executes a single fixture against one runner. Every runner is
// held to the same expected output; the runner only decides how the source is
// executed.
func RunFixtureOn(ctx context.Context, f *Fixture, r Runner) *TestResult {
	start := time.Now()
	res := &TestResult{
		Name:        f.Name,
		Description: f.Description,
		Path:        f.Path,
		Runner:      r,
	}

	if os.Getenv("DB_DSN_SQLITE_TEST") == "" {
		os.Setenv("DB_DSN_SQLITE_TEST", "sqlite://file:phpscript-test?mode=memory&cache=shared")
	}

	if ctx.Value(tenantKey) == nil {
		ctx = context.WithValue(ctx, tenantKey, "acme")
	}

	var (
		out      string
		runErr   error
		headers  http.Header
		hostFail string
	)

	switch r {
	case RunnerFlatstack:
		var reqCtx runner.Context
		out, reqCtx, runErr = executeFlatstack(ctx, f)
		headers = reqCtx.ResponseHeaders()
	case RunnerPHP:
		php, hostErr := executePHP(ctx, f)
		if hostErr != nil {
			res.Skipped = errors.Is(hostErr, ErrRunnerUnavailable)
			res.FailureReason = hostErr.Error()
			res.Error = hostErr.Error()
			res.DurationMs = time.Since(start).Milliseconds()
			return res
		}
		out = php.Stdout
		runErr = phpFatal(php.Stderr)
		hostFail = php.diagnostics()
	default:
		var reqCtx runner.Context
		out, reqCtx, runErr = executeFixturePHP(ctx, f)
		headers = reqCtx.ResponseHeaders()
	}

	res.GotOutput = out
	res.WantOutput = strings.TrimSuffix(f.Expected, "\n")
	res.DurationMs = time.Since(start).Milliseconds()

	if f.Error != "" {
		if runErr == nil {
			res.FailureReason = fmt.Sprintf("expected uncaught error containing %q, but execution succeeded with output %q", f.Error, out)
			return res
		}
		if !errorChainContains(runErr, f.Error) {
			res.FailureReason = fmt.Sprintf("uncaught error %q does not contain expected substring %q", runErr.Error(), f.Error)
			res.Error = runErr.Error()
			return res
		}
	} else if runErr != nil {
		res.FailureReason = fmt.Sprintf("unexpected uncaught error: %v", runErr)
		res.Error = runErr.Error()
		return res
	}

	// Response headers are staged by the host, so only an in-process runner can
	// be held to them.
	if headers != nil {
		for k, wantVal := range f.Response.Headers {
			gotVal := headers.Get(k)
			if gotVal != wantVal {
				res.FailureReason = fmt.Sprintf("header mismatch for %s: got %q, want %q", k, gotVal, wantVal)
				return res
			}
		}
	}

	gotTrim := strings.TrimSuffix(out, "\n")
	if gotTrim != res.WantOutput {
		res.FailureReason = fmt.Sprintf("output mismatch:\n  got:  %q\n  want: %q", gotTrim, res.WantOutput)
		if hostFail != "" {
			res.FailureReason += "\n  " + hostFail
		}
		return res
	}

	res.Passed = true
	return res
}

func (f *Fixture) program() (*model.Program, error) {
	if f.parsed != nil || f.parsedErr != nil {
		return f.parsed, f.parsedErr
	}
	f.parsed, f.parsedErr = parser.Parse(f.PHP)
	return f.parsed, f.parsedErr
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

	prog, err := f.program()
	if err != nil {
		return "Internal Server Error", runner.Context{}, err
	}

	if f.cleanState() {
		// A clean state is a new runtime, not only an empty cache. A runtime
		// keeps every declaration it hoisted out of an include, and this
		// fixture holds the runtime for the whole run, so reusing one carries
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
		// RegisterFS rebinds them to the fixture, so it has to run after it.
		stdlib.Register(rt)
		stdlib.RegisterFS(rt, f.RootDir())
		rt.FreezeStdlib()
	}
	// The harness parses and runs the fixture directly rather than going
	// through LoadFile, so nothing else sets the entrypoint and __FILE__ and
	// __DIR__ would both be empty. An empty __DIR__ silently turns
	// __DIR__ . "/x" into "/x", which is then rejected as escaping the root: a
	// confusing two-step failure a long way from its cause.
	rt.UpdateFilename(f.Path)
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

// executeFlatstack mirrors executeFixturePHP on the flat bytecode runtime, so
// both in-process runners are held to the same host behavior: an uncaught
// error produces the host error body, and the fixture request is registered.
func executeFlatstack(ctx context.Context, f *Fixture) (string, runner.Context, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	prog, err := f.program()
	if err != nil {
		return "Internal Server Error", runner.Context{}, err
	}

	var out strings.Builder
	reqCtx := buildFixtureRequestContext(f)
	if f.flatRT == nil {
		// A nil cache is caching off, which is what SetNoCache asks for; both
		// cache types answer a miss for nil rather than needing a branch here.
		flatCache, flatExpr := f.flatCaches(ctx)
		f.flatRT = newFlatstackTestRuntime(&out, f.runnerOptions(), f.RootDir(), f.Path, flatCache, flatExpr)
		f.flatRT.FreezeStdlib()
	} else {
		f.flatRT.ResetSession(&out, f.stdin())
	}
	f.flatRT.SetContext(ctx)
	reqCtx.Register(f.flatRT)

	if err := f.includePrelude(f.flatRT); err != nil {
		return "Internal Server Error", reqCtx, err
	}

	if f.cleanState() {
		defer func() { f.flatRT = nil }()
	}

	if err := f.flatRT.Run(prog); err != nil {
		if _, ok := flatstack.IsExit(err); ok {
			return out.String(), reqCtx, nil
		}
		return "Internal Server Error", reqCtx, err
	}
	return out.String(), reqCtx, nil
}

func newFlatstackTestRuntime(out *strings.Builder, options flatstack.Options, rootDir, entrypoint string, flatIncludeCache *flatstack.IncludeCache, flatExpr *flatstack.ExprCache) *flatstack.Runtime {
	runtime := flatstack.New(out, options)
	runtime.SetIncludeCache(flatIncludeCache)
	runtime.SetExprCache(flatExpr)
	runtime.RegisterConstructor("Storage", NewStorage)
	runtime.RegisterConstructor("FailStorage", NewFailStorage)
	registerPanicBindings(runtime)
	stdlib.Register(runtime)
	stdlib.RegisterFS(runtime, rootDir)
	runtime.UpdateFilename(entrypoint)
	return runtime
}

func buildFixtureRequestContext(f *Fixture) runner.Context {
	reqCtx := runner.NewContext()
	reqCtx.Get = f.Request.Get
	reqCtx.Post = f.Request.Post
	reqCtx.Cookie = f.Request.Cookie
	reqCtx.Env = f.Request.Env
	reqCtx.Headers = f.Request.Headers

	if reqCtx.Get == nil {
		reqCtx.Get = make(map[string]string)
	}
	if reqCtx.Post == nil {
		reqCtx.Post = make(map[string]string)
	}
	if reqCtx.Cookie == nil {
		reqCtx.Cookie = make(map[string]string)
	}
	if reqCtx.Env == nil {
		reqCtx.Env = make(map[string]string)
	}
	if reqCtx.Headers == nil {
		reqCtx.Headers = make(map[string]string)
	}
	if reqCtx.Server == nil {
		reqCtx.Server = make(map[string]string)
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

func parseArgs(raw any, path string) []string {
	scriptName := "phpscript"
	if path != "" {
		scriptName = filepath.Base(path)
	}
	argv := []string{scriptName}

	switch v := raw.(type) {
	case []any:
		for _, arg := range v {
			argv = append(argv, fmt.Sprint(arg))
		}
	case map[string]any:
		// Map representation e.g. args: {name: Alice} -> ["script", "Alice"] or values sorted by key
		var keys []string
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			argv = append(argv, fmt.Sprint(v[k]))
		}
	case string:
		argv = append(argv, v)
	default:
		rv := reflect.ValueOf(raw)
		if rv.Kind() == reflect.Slice {
			for i := 0; i < rv.Len(); i++ {
				argv = append(argv, fmt.Sprint(rv.Index(i).Interface()))
			}
		}
	}
	return argv
}

// WriteJSONReport outputs a JSON report of the test results.
func WriteJSONReport(w io.Writer, results []*TestResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{
		"total":   len(results),
		"results": results,
	})
}

var htmlReportTemplate = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>PHPScript Test Results</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 2rem; background: #0f172a; color: #f8fafc; }
		h1 { margin-bottom: 0.5rem; }
		.summary { font-size: 1.1rem; margin-bottom: 2rem; }
		.pass-count { color: #4ade80; font-weight: bold; }
		.fail-count { color: #f87171; font-weight: bold; }
		table { width: 100%; border-collapse: collapse; background: #1e293b; border-radius: 8px; overflow: hidden; }
		th, td { padding: 12px 16px; text-align: left; border-bottom: 1px solid #334155; }
		th { background: #334155; color: #94a3b8; text-transform: uppercase; font-size: 0.8rem; letter-spacing: 0.05em; }
		.status { font-weight: bold; padding: 4px 8px; border-radius: 4px; display: inline-block; font-size: 0.85rem; }
		.status.PASS { background: #166534; color: #4ade80; }
		.status.FAIL { background: #991b1b; color: #f87171; }
		.details { font-family: monospace; font-size: 0.9rem; color: #cbd5e1; white-space: pre-wrap; margin-top: 4px; }
	</style>
</head>
<body>
	<h1>PHPScript Test Results</h1>
	<div class="summary">
		Total: {{.Total}} | <span class="pass-count">Passed: {{.Passed}}</span> | <span class="fail-count">Failed: {{.Failed}}</span>
	</div>
	<table>
		<thead>
			<tr>
				<th>Status</th>
				<th>Name</th>
				<th>Path</th>
				<th>Duration</th>
				<th>Details</th>
			</tr>
		</thead>
		<tbody>
			{{range .Results}}
			<tr>
				<td><span class="status {{if .Passed}}PASS{{else}}FAIL{{end}}">{{if .Passed}}PASS{{else}}FAIL{{end}}</span></td>
				<td><strong>{{.Name}}</strong><br><small style="color:#94a3b8">{{.Description}}</small></td>
				<td><code>{{.Path}}</code></td>
				<td>{{.DurationMs}} ms</td>
				<td>{{if .FailureReason}}<div class="details">{{.FailureReason}}</div>{{else}}n/a{{end}}</td>
			</tr>
			{{end}}
		</tbody>
	</table>
</body>
</html>`))

// WriteHTMLReport outputs an HTML report of the test results.
func WriteHTMLReport(w io.Writer, results []*TestResult) error {
	var passed, failed int
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	return htmlReportTemplate.Execute(w, map[string]any{
		"Total":   len(results),
		"Passed":  passed,
		"Failed":  failed,
		"Results": results,
	})
}
