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
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"

	"gopkg.in/yaml.v3"
)

//go:embed all:fixtures
var fixturesFS embed.FS

var phpFS fs.FS
var phpFSOnce sync.Once

var includeCache = runner.NewIncludeCache()
var exprCache = runner.NewExprCache()

var flatIncludeCache = flatstack.NewIncludeCache()
var flatExprCache = flatstack.NewExprCache()

// ResetCaches clears all global shared caches between test suites.
func ResetCaches() {
	includeCache.Clear()
	exprCache.Clear()
	flatIncludeCache.Clear()
	flatExprCache.Clear()
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
	Error       string          `yaml:"error"`     // optional: expected uncaught error substring
	Stdin       string          `yaml:"stdin"`     // optional: top-level stdin contents
	Flatstack   bool            `yaml:"flatstack"` // optional: also test on flatstack bytecode
	Request     FixtureRequest  `yaml:"request"`
	Response    FixtureResponse `yaml:"response"`

	PHP      string `yaml:"-"`
	Expected string `yaml:"-"`
	Path     string `yaml:"-"`
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
	FlatstackUsed bool   `json:"flatstack_used,omitempty"`
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
	return fx, nil
}

// RunFixture executes a single fixture against the runner (and flatstack if requested).
func RunFixture(ctx context.Context, f *Fixture) *TestResult {
	start := time.Now()
	res := &TestResult{
		Name:        f.Name,
		Description: f.Description,
		Path:        f.Path,
	}

	if os.Getenv("DB_DSN_SQLITE_TEST") == "" {
		os.Setenv("DB_DSN_SQLITE_TEST", "sqlite://file:phpscript-test?mode=memory&cache=shared")
	}

	if ctx.Value(tenantKey) == nil {
		ctx = context.WithValue(ctx, tenantKey, "acme")
	}

	out, reqCtx, runErr := executeFixturePHP(ctx, f)
	res.GotOutput = out
	res.WantOutput = strings.TrimSuffix(f.Expected, "\n")
	res.DurationMs = time.Since(start).Milliseconds()

	if f.Error != "" {
		if runErr == nil {
			res.Passed = false
			res.FailureReason = fmt.Sprintf("expected uncaught error containing %q, but execution succeeded with output %q", f.Error, out)
			return res
		}
		if !errorChainContains(runErr, f.Error) {
			res.Passed = false
			res.FailureReason = fmt.Sprintf("uncaught error %q does not contain expected substring %q", runErr.Error(), f.Error)
			res.Error = runErr.Error()
			return res
		}
	} else if runErr != nil {
		res.Passed = false
		res.FailureReason = fmt.Sprintf("unexpected uncaught error: %v", runErr)
		res.Error = runErr.Error()
		return res
	}

	// Verify response.headers if specified
	if len(f.Response.Headers) > 0 {
		stagedHeaders := reqCtx.ResponseHeaders()
		for k, wantVal := range f.Response.Headers {
			gotVal := stagedHeaders.Get(k)
			if gotVal != wantVal {
				res.Passed = false
				res.FailureReason = fmt.Sprintf("header mismatch for %s: got %q, want %q", k, gotVal, wantVal)
				return res
			}
		}
	}

	// Compare stdout
	gotTrim := strings.TrimSuffix(out, "\n")
	if gotTrim != res.WantOutput {
		res.Passed = false
		res.FailureReason = fmt.Sprintf("output mismatch:\n  got:  %q\n  want: %q", gotTrim, res.WantOutput)
		return res
	}

	// If flatstack execution is requested, also run via Flatstack
	if f.Flatstack {
		res.FlatstackUsed = true
		flatOut, flatErr := executeFlatstack(ctx, f)
		if f.Error != "" {
			if flatErr == nil {
				res.Passed = false
				res.FailureReason = fmt.Sprintf("[flatstack] expected error containing %q, but succeeded", f.Error)
				return res
			}
		} else if flatErr != nil {
			res.Passed = false
			res.FailureReason = fmt.Sprintf("[flatstack] unexpected error: %v", flatErr)
			return res
		} else {
			flatTrim := strings.TrimSuffix(flatOut, "\n")
			if flatTrim != res.WantOutput {
				res.Passed = false
				res.FailureReason = fmt.Sprintf("[flatstack] output mismatch:\n  got:  %q\n  want: %q", flatTrim, res.WantOutput)
				return res
			}
		}
	}

	res.Passed = true
	return res
}

func executeFixturePHP(ctx context.Context, f *Fixture) (string, runner.Context, error) {
	prog, err := parser.Parse(f.PHP)
	if err != nil {
		return "Internal Server Error", runner.Context{}, err
	}

	var out strings.Builder
	reqCtx := buildFixtureRequestContext(f)

	stdinContent := f.Request.Stdin
	if stdinContent == "" {
		stdinContent = f.Stdin
	}

	options := runner.Options{
		RootFS: testPHPFS(),
	}
	if stdinContent != "" {
		options.Stdin = strings.NewReader(stdinContent)
	}

	rt := runner.New(&out, options)
	rt.SetIncludeCache(includeCache)
	rt.SetExprCache(exprCache)
	rt.SetContext(ctx)

	rt.RegisterConstructor("Storage", NewStorage)
	rt.RegisterConstructor("FailStorage", NewFailStorage)
	stdlib.RegisterFS(rt, ".")
	stdlib.Register(rt)

	reqCtx.Register(rt)

	if err := rt.Run(prog); err != nil {
		if _, ok := runner.IsExit(err); ok {
			return out.String(), reqCtx, nil
		}
		return "Internal Server Error", reqCtx, err
	}

	return out.String(), reqCtx, nil
}

func executeFlatstack(ctx context.Context, f *Fixture) (string, error) {
	prog, err := parser.Parse(f.PHP)
	if err != nil {
		return "", err
	}
	if err := flatstack.Supports(prog); err != nil {
		return "", fmt.Errorf("flatstack unsupported: %w", err)
	}

	var out strings.Builder
	stdinContent := f.Request.Stdin
	if stdinContent == "" {
		stdinContent = f.Stdin
	}

	runtime := newFlatstackTestRuntime(&out, ctx, strings.NewReader(stdinContent))
	if err := runtime.Run(prog); err != nil {
		if _, ok := flatstack.IsExit(err); ok {
			return out.String(), nil
		}
		return out.String(), err
	}

	return out.String(), nil
}

func newFlatstackTestRuntime(out *strings.Builder, ctx context.Context, input ...*strings.Reader) *flatstack.Runtime {
	options := flatstack.Options{RootFS: testPHPFS()}
	if len(input) > 0 {
		options.Stdin = input[0]
	}
	runtime := flatstack.New(out, options)
	runtime.SetIncludeCache(flatIncludeCache)
	runtime.SetExprCache(flatExprCache)
	runtime.SetContext(context.WithValue(ctx, tenantKey, "acme"))
	runtime.RegisterConstructor("Storage", NewStorage)
	runtime.RegisterConstructor("FailStorage", NewFailStorage)
	stdlib.RegisterFS(runtime, ".")
	stdlib.Register(runtime)
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
				<td>{{if .FailureReason}}<div class="details">{{.FailureReason}}</div>{{else}}—{{end}}</td>
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
