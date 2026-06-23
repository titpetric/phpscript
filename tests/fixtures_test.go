// Package tests holds black-box, end-to-end tests that drive the runner the way
// a host application would: registering Go-side capabilities (constructors,
// methods, context) and exercising them from PHP source.
//
// Tests are data-driven .phpt fixtures under fixtures/. Each fixture is:
//
//	<yaml metadata>     # required: name, description; optional: error
//	---
//	<php source>
//	---
//	<expected output>
//
// When the metadata declares an `error`, the fixture expects execution to fail
// with an error message containing that substring (and produce no output);
// otherwise the program's echo output must equal the expected-output section.
package tests

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"os"
	"runtime/pprof"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"gopkg.in/yaml.v3"
)

//go:embed all:fixtures
var fixturesFS embed.FS

var phpFS fs.FS
var includeCache = runner.NewIncludeCache()
var exprCache = runner.NewExprCache()

// ---------------------------------------------------------------------------
// Host-provided capability surfaced to PHP
// ---------------------------------------------------------------------------

// Storage is a host capability exposed to PHP. The constructor takes a
// context.Context (auto-injected by the runner) and may fail; its methods read
// and write key/value pairs. This is the "bring your own type" bridge:
//
//	PHP:  $storage = new Storage;          // == storage, err := NewStorage(ctx)
//	      $storage->set("k", "v");         //    storage.Set("k", "v")
//	      $val = $storage->get("k");       //    val, err := storage.Get("k")
//	      echo $val;                       //    print the assigned value
//
// Every method takes a context.Context as its first parameter; the runner
// auto-injects the runtime context, so PHP calls them without it
// (`$storage->get("k")` == storage.Get(ctx, "k")).
type Storage interface {
	Set(ctx context.Context, key, value string)
	Get(ctx context.Context, key string) (Record, error)
	All(ctx context.Context) ([]Record, error)
	Len() int64
	Tenant() string
}

// Record is a rich value type returned to PHP. Its exported fields are read from
// PHP via property access (`$rec->key`, `$rec->value`), matched case-insensitively.
type Record struct {
	Key   string
	Value string
}

// memStorage is an in-memory Storage implementation.
type memStorage struct {
	data   map[string]string
	tenant string
}

func (s *memStorage) Set(_ context.Context, key, value string) { s.data[key] = value }

// Get returns (Record, error): a rich struct value (not a scalar). The error is
// omitted on the PHP side (handled as a throw); the Record is assigned to the
// PHP variable, whose fields are then read with `->`.
func (s *memStorage) Get(_ context.Context, key string) (Record, error) {
	v, ok := s.data[key]
	if !ok {
		return Record{}, errors.New("storage: missing key " + key)
	}
	return Record{Key: key, Value: v}, nil
}

// All returns a list of rich types ([]Record), key-sorted for determinism, so
// PHP can foreach over a Go slice and read struct fields on each element.
func (s *memStorage) All(_ context.Context) ([]Record, error) {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Record, 0, len(keys))
	for _, k := range keys {
		out = append(out, Record{Key: k, Value: s.data[k]})
	}
	return out, nil
}

func (s *memStorage) Len() int64     { return int64(len(s.data)) }
func (s *memStorage) Tenant() string { return s.tenant }

// ctxKey is the context key used to thread request-scoped data into constructors.
type ctxKey string

const tenantKey ctxKey = "tenant"

// NewStorage is the constructor registered for `new Storage`. Its first
// parameter is a context.Context, filled in automatically by the runner, so PHP
// calls `new Storage` with no arguments.
func NewStorage(ctx context.Context) (Storage, error) {
	if ctx == nil {
		return nil, errors.New("storage: nil context")
	}
	tenant, _ := ctx.Value(tenantKey).(string)
	return &memStorage{data: map[string]string{}, tenant: tenant}, nil
}

// NewFailStorage is a constructor that always fails, used to exercise the
// thrown-error path of `new`.
func NewFailStorage(ctx context.Context) (Storage, error) {
	return nil, errors.New("boom")
}

// ---------------------------------------------------------------------------
// Fixture format + harness
// ---------------------------------------------------------------------------

// fixture is the parsed form of a .phpt file.
type fixture struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Error       string `yaml:"error"` // optional: expected error substring

	PHP      string `yaml:"-"`
	Expected string `yaml:"-"`
}

// parseFixture splits a .phpt file into its three sections and unmarshals the
// YAML metadata. Sections are separated by lines containing only `---`.
func parseFixture(data []byte) (fixture, error) {
	parts := strings.SplitN(string(data), "\n---\n", 3)
	if len(parts) != 3 {
		return fixture{}, errors.New("malformed .phpt: want <yaml>---<php>---<output>")
	}
	var f fixture
	if err := yaml.Unmarshal([]byte(parts[0]), &f); err != nil {
		return fixture{}, err
	}
	if f.Name == "" {
		return fixture{}, errors.New("fixture metadata missing required 'name'")
	}
	if f.Description == "" {
		return fixture{}, errors.New("fixture metadata missing required 'description'")
	}
	f.PHP = strings.TrimPrefix(parts[1], "\n")
	f.Expected = parts[2]
	return f, nil
}

// runFixture executes the fixture's PHP the way the HTTP host does: output is
// buffered and, on an uncaught error, discarded in favour of an "Internal
// Server Error" body. The underlying error is returned too so a fixture can
// additionally assert its message via the `error` metadata field.
func runFixture(ctx context.Context, f fixture) (string, error) {
	prog, err := parser.Parse(f.PHP)
	if err != nil {
		return "Internal Server Error", err
	}

	var out strings.Builder
	rt := newTestRuntime(&out, ctx)

	/*
		rt.RegisterFunc("sprintf", fmt.Sprintf)
		rt.RegisterFunc("rtrim", strings.TrimRight)
		rt.RegisterFunc("is_string", func(in any) bool {
			_, ok := in.(string)
			return ok
		})
		rt.RegisterFunc("empty", func(in any) bool {
			return in == reflect.Zero(reflect.TypeOf(in)).Interface()
		}) */

	if err := rt.Run(prog); err != nil {
		return "Internal Server Error", err
	}
	return out.String(), nil
}

func testPHPFS() fs.FS {
	if phpFS == nil {
		var err error
		phpFS, err = fs.Sub(fixturesFS, "fixtures")
		if err != nil {
			panic(err)
		}
	}
	return phpFS
}

func newTestRuntime(out *strings.Builder, ctx context.Context) *runner.Runtime {
	rt := runner.New(out, runner.Options{RootFS: testPHPFS()})
	rt.SetIncludeCache(includeCache)
	rt.SetExprCache(exprCache)
	rt.SetContext(context.WithValue(ctx, tenantKey, "acme"))
	rt.RegisterConstructor("Storage", NewStorage)
	rt.RegisterConstructor("FailStorage", NewFailStorage)
	stdlib.RegisterDatabase(rt)
	stdlib.RegisterFS(rt, ".")
	stdlib.Register(rt)
	return rt
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

// TestFixtures discovers every fixtures/*.phpt file, runs it, and asserts the
// program output (or the expected error). It also prints a summary table.
func TestFixtures(t *testing.T) {
	entries, err := fs.ReadDir(fixturesFS, "fixtures")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}

	type result struct {
		name   string
		passed bool
	}
	var results []result

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".phpt") {
			continue
		}
		data, err := fixturesFS.ReadFile("fixtures/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		f, err := parseFixture(data)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}

		passed := t.Run(f.Name, func(t *testing.T) {
			out, runErr := runFixture(t.Context(), f)

			// The `error` field asserts an *uncaught* error surfaced to the
			// host: it must occur and its message must contain the substring.
			if f.Error != "" {
				if runErr == nil {
					t.Fatalf("expected error containing %q, got output %q", f.Error, out)
				}
				if !errorChainContains(runErr, f.Error) {
					t.Fatalf("error %v does not contain %q", runErr, f.Error)
				}
			} else if runErr != nil {
				t.Fatalf("unexpected uncaught error: %v", runErr)
			}

			// Output (host body) is always compared against the expected section.
			want := strings.TrimRight(f.Expected, "\n")
			got := strings.TrimRight(out, "\n")
			if got != want {
				t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
			}
		})

		results = append(results, result{f.Name, passed})
	}

	// Summary table.
	t.Log("Fixture summary:")
	for _, r := range results {
		status := "PASS"
		if !r.passed {
			status = "FAIL"
		}
		t.Logf("  [%s] %s", status, r.name)
	}
}

func BenchmarkMinitpl(b *testing.B) {
	srcBytes, err := fixturesFS.ReadFile("fixtures/test-minitpl.php")
	if err != nil {
		b.Fatal(err)
	}
	src := string(srcBytes)

	b.Run("parse", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := parser.Parse(src); err != nil {
				b.Fatal(err)
			}
		}
	})

	prog, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("runtime_setup", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var out strings.Builder
			_ = newTestRuntime(&out, b.Context())
		}
	})

	b.Run("run_preparsed", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var out strings.Builder
			rt := newTestRuntime(&out, b.Context())
			if err := rt.Run(prog); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("end_to_end_accounting", func(b *testing.B) {
		b.ReportAllocs()
		var parseDur, setupDur, runDur time.Duration
		for b.Loop() {
			start := time.Now()
			prog, err := parser.Parse(src)
			parseDur += time.Since(start)
			if err != nil {
				b.Fatal(err)
			}

			var out strings.Builder
			start = time.Now()
			rt := newTestRuntime(&out, b.Context())
			setupDur += time.Since(start)

			start = time.Now()
			if err := rt.Run(prog); err != nil {
				b.Fatal(err)
			}
			runDur += time.Since(start)
		}
		b.ReportMetric(float64(parseDur.Nanoseconds())/float64(b.N), "parse_ns/op")
		b.ReportMetric(float64(setupDur.Nanoseconds())/float64(b.N), "setup_ns/op")
		b.ReportMetric(float64(runDur.Nanoseconds())/float64(b.N), "run_ns/op")
	})
}

func TestMinitplProfiles(t *testing.T) {
	if os.Getenv("PHPSCRIPT_WRITE_PROFILES") != "1" {
		t.Skip("set PHPSCRIPT_WRITE_PROFILES=1 to write cpu/memory profiles")
	}
	srcBytes, err := fixturesFS.ReadFile("fixtures/test-minitpl.php")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	cpu, err := os.Create("minitpl.cpu.pprof")
	if err != nil {
		t.Fatal(err)
	}
	defer cpu.Close()
	if err := pprof.StartCPUProfile(cpu); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		rt := newTestRuntime(&out, t.Context())
		if err := rt.Run(prog); err != nil {
			t.Fatal(err)
		}
	}
	pprof.StopCPUProfile()

	mem, err := os.Create("minitpl.mem.pprof")
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	if err := pprof.WriteHeapProfile(mem); err != nil {
		t.Fatal(err)
	}
}

// oneLine collapses a (possibly multi-line, folded YAML) description into a
// single line for the summary table.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
