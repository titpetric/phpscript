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
	"github.com/titpetric/phpscript/stdlib"
)

//go:embed all:fixtures
var fixturesFS embed.FS

var phpFS fs.FS

var phpFSOnce sync.Once

var exprCache = runner.NewExprCache()

var flatExprCache = flatstack.NewExprCache()

// An include cache is keyed by the path as the script wrote it, so one cache
// shared across fixture areas would serve syntax/code/functions.php to a
// fixture in another area that includes its own code/functions.php. The caches
// are therefore per include root, which is the directory holding the fixture.
var (
	includeCaches     sync.Map // string -> *runner.IncludeCache
	flatIncludeCaches sync.Map // string -> *flatstack.IncludeCache
)

func includeCacheFor(dir string) *runner.IncludeCache {
	if cache, ok := includeCaches.Load(dir); ok {
		return cache.(*runner.IncludeCache)
	}
	cache, _ := includeCaches.LoadOrStore(dir, runner.NewIncludeCache())
	return cache.(*runner.IncludeCache)
}

func flatIncludeCacheFor(dir string) *flatstack.IncludeCache {
	if cache, ok := flatIncludeCaches.Load(dir); ok {
		return cache.(*flatstack.IncludeCache)
	}
	cache, _ := flatIncludeCaches.LoadOrStore(dir, flatstack.NewIncludeCache())
	return cache.(*flatstack.IncludeCache)
}

// ResetCaches clears all global shared caches between test suites.
func ResetCaches() {
	exprCache.Clear()
	flatExprCache.Clear()
	includeCaches.Range(func(key, value any) bool {
		value.(*runner.IncludeCache).Clear()
		includeCaches.Delete(key)
		return true
	})
	flatIncludeCaches.Range(func(key, value any) bool {
		value.(*flatstack.IncludeCache).Clear()
		flatIncludeCaches.Delete(key)
		return true
	})
}

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
	Runner      FixtureRunners  `yaml:"runner"`  // optional: runners the fixture opts out of
	Request     FixtureRequest  `yaml:"request"`
	Response    FixtureResponse `yaml:"response"`

	PHP      string `yaml:"-"`
	Expected string `yaml:"-"`
	Path     string `yaml:"-"`

	rootFS             fs.FS
	privateInclude     *runner.IncludeCache
	privateFlatInclude *flatstack.IncludeCache
	mu                 sync.Mutex
	parsed             *model.Program
	parsedErr          error
	interp             *runner.Runtime
	flatRT             *flatstack.Runtime
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
	return f.Root != ""
}

// includeCache is the include cache for the fixture's root. A fixture reaching
// a real tree gets a private one rather than sharing by directory name.
func (f *Fixture) includeCache() *runner.IncludeCache {
	if f.realRoot() {
		if f.privateInclude == nil {
			f.privateInclude = runner.NewIncludeCache()
		}
		return f.privateInclude
	}
	return includeCacheFor(f.RootDir())
}

// flatIncludeCache is includeCache for the bytecode runtime.
func (f *Fixture) flatIncludeCache() *flatstack.IncludeCache {
	if f.realRoot() {
		if f.privateFlatInclude == nil {
			f.privateFlatInclude = flatstack.NewIncludeCache()
		}
		return f.privateFlatInclude
	}
	return flatIncludeCacheFor(f.RootDir())
}

// runnerOptions returns the fixture's options: frontmatter with the
// harness-owned fields filled in.
func (f *Fixture) runnerOptions() runner.Options {
	options := f.Options
	options.RootFS = f.rootFS
	if f.realRoot() {
		// A fixture that names a root wants the real filesystem: it is reaching
		// for a tree phpscript does not embed, a vendor directory being the
		// motivating case.
		options.RootFS = os.DirFS(f.RootDir())
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

	var out strings.Builder
	reqCtx := buildFixtureRequestContext(f)
	if f.interp == nil {
		options := f.runnerOptions()
		rt := runner.New(&out, options)
		rt.SetIncludeCache(f.includeCache())
		rt.SetExprCache(exprCache)
		rt.RegisterConstructor("Storage", NewStorage)
		rt.RegisterConstructor("FailStorage", NewFailStorage)
		registerPanicBindings(rt)
		stdlib.RegisterFS(rt, f.RootDir())
		stdlib.Register(rt)
		// The harness parses and runs the fixture directly rather than going
		// through LoadFile, so nothing else sets the entrypoint and __FILE__
		// and __DIR__ would both be empty. An empty __DIR__ silently turns
		// __DIR__ . "/x" into "/x", which is then rejected as escaping the
		// root: a confusing two-step failure a long way from its cause.
		rt.UpdateFilename(f.Path)
		rt.FreezeStdlib()
		f.interp = rt
	} else {
		f.interp.ResetSession(&out, f.stdin())
	}
	f.interp.SetContext(ctx)
	reqCtx.Register(f.interp)

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
		f.flatRT = newFlatstackTestRuntime(&out, f.runnerOptions(), f.RootDir(), f.Path, f.flatIncludeCache())
		f.flatRT.FreezeStdlib()
	} else {
		f.flatRT.ResetSession(&out, f.stdin())
	}
	f.flatRT.SetContext(ctx)
	reqCtx.Register(f.flatRT)

	if err := f.flatRT.Run(prog); err != nil {
		if _, ok := flatstack.IsExit(err); ok {
			return out.String(), reqCtx, nil
		}
		return "Internal Server Error", reqCtx, err
	}
	return out.String(), reqCtx, nil
}

func newFlatstackTestRuntime(out *strings.Builder, options flatstack.Options, rootDir, entrypoint string, flatIncludeCache *flatstack.IncludeCache) *flatstack.Runtime {
	runtime := flatstack.New(out, options)
	runtime.SetIncludeCache(flatIncludeCache)
	runtime.SetExprCache(flatExprCache)
	runtime.RegisterConstructor("Storage", NewStorage)
	runtime.RegisterConstructor("FailStorage", NewFailStorage)
	registerPanicBindings(runtime)
	stdlib.RegisterFS(runtime, rootDir)
	stdlib.Register(runtime)
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
