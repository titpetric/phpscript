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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/database"
	"github.com/titpetric/phpscript/stdlib/mail"
)

// ---------------------------------------------------------------------------
// Fixture format + harness
// ---------------------------------------------------------------------------

// fixture is the parsed form of a .phpt file.
type fixture struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Error       string `yaml:"error"` // optional: expected error substring
	Stdin       string `yaml:"stdin"` // optional: runtime STDIN contents

	PHP      string `yaml:"-"`
	Expected string `yaml:"-"`
}

// parseFixture splits a .phpt file into its three sections and unmarshals the
// YAML metadata. A line holding only `---` divides one section from the next.
func parseFixture(data []byte) (fixture, error) {
	parts := strings.SplitN(string(data), "\n---\n", 3)
	if len(parts) != 3 {
		return fixture{}, errors.New("malformed .phpt: want <yaml>---<php>---<output>")
	}
	var f fixture
	if err := unmarshalHeader([]byte(parts[0]), &f); err != nil {
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

// runFixture executes the fixture's PHP the way the HTTP host does: it
// buffers output and, on an uncaught error, discards it in favour of an
// "Internal Server Error" body. It returns the underlying error too, so a
// fixture can also assert its message through the `error` metadata field.
func runFixture(ctx context.Context, f fixture) (string, error) {
	prog, err := parser.Parse(f.PHP)
	if err != nil {
		return "Internal Server Error", err
	}

	var out strings.Builder
	rt := newTestRuntime(&out, ctx, strings.NewReader(f.Stdin))

	if err := rt.Run(prog); err != nil {
		if _, ok := runner.IsExit(err); ok {
			return out.String(), nil
		}
		return "Internal Server Error", err
	}
	return out.String(), nil
}

func newTestRuntime(out *strings.Builder, ctx context.Context, input ...*strings.Reader) *runner.Runtime {
	options := runner.Options{RootFS: testPHPFS()}
	if len(input) > 0 {
		options.Stdin = input[0]
	}
	rt := runner.New(out, options)
	rt.SetIncludeCache(runner.NewIncludeCache())
	rt.SetExprCache(runner.NewExprCache())
	rt.SetContext(context.WithValue(ctx, tenantKey, "acme"))
	rt.RegisterConstructor("Storage", NewStorage)
	rt.RegisterConstructor("FailStorage", NewFailStorage)
	stdlib.RegisterFS(rt, ".")
	stdlib.Register(rt)
	runner.NewContext().Register(rt)
	return rt
}

// TestFixtures discovers every fixtures/<area>/*.phpt file, runs it, and
// asserts the program output (or the expected error). It also prints a summary
// table, grouped the way the fixture tree is.
func TestFixtures(t *testing.T) {
	areas, err := embeddedFixtures()
	if err != nil {
		t.Fatalf("discover fixtures: %v", err)
	}
	if len(areas) == 0 {
		t.Fatal("no fixtures discovered")
	}

	teardown, err := setupAreas(t.Context(), areas, io.Discard)
	if err != nil {
		t.Fatalf("suite setup: %v", err)
	}
	t.Cleanup(func() {
		if err := teardown(); err != nil {
			t.Errorf("suite teardown: %v", err)
		}
	})

	type result struct {
		area   string
		name   string
		passed bool
	}
	var results []result

	for _, area := range areas {
		t.Run(area.Name, func(t *testing.T) {
			for _, fx := range area.Fixtures {
				passed := t.Run(fx.Name, func(t *testing.T) {
					res := RunFixture(t.Context(), fx)
					if !res.Passed {
						t.Fatalf("fixture failed: %s", res.FailureReason)
					}
				})
				results = append(results, result{area.Name, fx.Name, passed})
			}
		})
	}

	// Summary table.
	t.Log("Fixture summary:")
	for _, r := range results {
		status := "PASS"
		if !r.passed {
			status = "FAIL"
		}
		t.Logf("  [%s] %s/%s", status, r.area, r.name)
	}
}

var bindingBenchmarkSink int

// BenchmarkGoBindingHTTP compares the same constructor and API calls made by a
// native Go HTTP handler and by a per-request PHP runtime. Source parsing sits
// outside the timed region, and each iteration reuses one prebuilt request.
// The timed region covers recorder allocation, runtime setup, reflective
// binding dispatch, execution and response writing, where each applies.
func BenchmarkGoBindingHTTP(b *testing.B) {
	prog, err := parser.Parse(`<?php
$storage = new Storage;
$storage->set("color", "blue");
$record = $storage->get("color");
echo $storage->tenant() . ":" . $record->value;
`)
	if err != nil {
		b.Fatal(err)
	}
	sharedExprCache := runner.NewExprCache()
	flatExprCache := flatstack.NewExprCache()
	if err := flatstack.Supports(prog); err != nil {
		b.Fatalf("flatstack benchmark would fall back: %v", err)
	}

	goHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storage, err := NewStorage(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		storage.Set(r.Context(), "color", "blue")
		record, err := storage.Get(r.Context(), "color")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(storage.Tenant() + ":" + record.Value))
	})

	phpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt := runner.New(w, runner.Options{})
		rt.SetExprCache(sharedExprCache)
		rt.SetContext(r.Context())
		rt.RegisterConstructor("Storage", NewStorage)
		if err := rt.Run(prog); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	flatHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt := flatstack.New(w, flatstack.Options{})
		rt.SetExprCache(flatExprCache)
		rt.SetContext(r.Context())
		rt.RegisterConstructor("Storage", NewStorage)
		if err := rt.Run(prog); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	request := httptest.NewRequest(http.MethodGet, "/binding", nil)
	request = request.WithContext(context.WithValue(request.Context(), tenantKey, "acme"))
	const expected = "acme:blue"

	benchmarks := []struct {
		name    string
		handler http.Handler
	}{
		{name: "go_handler", handler: goHandler},
		{name: "php_vm_handler", handler: phpHandler},
		{name: "flatstack_handler", handler: flatHandler},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			check := httptest.NewRecorder()
			benchmark.handler.ServeHTTP(check, request)
			if check.Code != http.StatusOK || check.Body.String() != expected {
				b.Fatalf("status/body = %d/%q, want %d/%q", check.Code, check.Body.String(), http.StatusOK, expected)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				response := httptest.NewRecorder()
				benchmark.handler.ServeHTTP(response, request)
				bindingBenchmarkSink = response.Body.Len()
			}
		})
	}
}

// oneLine collapses a (possibly multi-line, folded YAML) description into a
// single line for the summary table.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// TestFixtureRequest covers the frontmatter block a fixture describes its
// superglobals with. Every map keys on the name PHP reads it under, so a
// fixture writes $_GET and $_POST as yaml and the harness copies them across
// without a translation table.
func TestFixtureRequest(t *testing.T) {
	fx, err := ParseFixture([]byte("name: request\ndescription: request block\nrequest:\n  get:\n    page: \"2\"\n  post:\n    name: alice\n  cookie:\n    sid: abc\n  env:\n    APP: test\n  headers:\n    X-Trace: t1\n  stdin: \"in\"\n  body: \"raw\"\n  args: [\"one\"]\n---\n<?php\n---\n"), "request.phpt")
	if err != nil {
		t.Fatal(err)
	}

	want := FixtureRequest{
		Args:    []any{"one"},
		Get:     map[string]string{"page": "2"},
		Post:    map[string]string{"name": "alice"},
		Cookie:  map[string]string{"sid": "abc"},
		Env:     map[string]string{"APP": "test"},
		Headers: map[string]string{"X-Trace": "t1"},
		Stdin:   "in",
		Body:    "raw",
	}
	if !reflect.DeepEqual(fx.Request, want) {
		t.Errorf("Request = %+v, want %+v", fx.Request, want)
	}
}

// TestFixtureResponse covers the headers a fixture holds the response to. The
// block is assertions and no input, so it carries only what a run compares
// against.
func TestFixtureResponse(t *testing.T) {
	fx, err := ParseFixture([]byte("name: response\ndescription: response block\nresponse:\n  headers:\n    Content-Type: text/plain\n---\n<?php\n---\n"), "response.phpt")
	if err != nil {
		t.Fatal(err)
	}

	want := FixtureResponse{Headers: map[string]string{"Content-Type": "text/plain"}}
	if !reflect.DeepEqual(fx.Response, want) {
		t.Errorf("Response = %+v, want %+v", fx.Response, want)
	}
}

// TestFixture_SetRootFS covers the filesystem a fixture's includes resolve
// against. The directory holding the .phpt is what every caller binds, so a
// relative require reaches the same file under all three runners.
func TestFixture_SetRootFS(t *testing.T) {
	fx, err := ParseFixture([]byte("name: rootfs\ndescription: include root\n---\n<?php\nrequire \"helper.php\";\necho helper();\n---\nhelped"), "rootfs.phpt")
	if err != nil {
		t.Fatal(err)
	}
	fx.SetRootFS(fstest.MapFS{
		"helper.php": &fstest.MapFile{Data: []byte("<?php\nfunction helper() { return \"helped\"; }\n")},
	})

	res := RunFixture(t.Context(), fx)
	if !res.Passed {
		t.Fatalf("fixture failed: %s", res.FailureReason)
	}
}

// TestShareSources covers the layer precompiling one directory compiles from.
// Every body in the directory sits in one filesystem, so one compile covers
// the whole area and no fixture in it compiles the tree again.
func TestShareSources(t *testing.T) {
	first := &Fixture{Name: "first", Path: "area/first.phpt", PHP: "<?php echo 1;"}
	second := &Fixture{Name: "second", Path: "area/second.phpt", PHP: "<?php echo 2;"}

	ShareSources([]*Fixture{first, second})

	for _, fx := range []*Fixture{first, second} {
		if got := len(fx.sources); got != 2 {
			t.Errorf("%s carries %d bodies, want 2", fx.Name, got)
		}
		if _, ok := fx.sources[first.sourceName()]; !ok {
			t.Errorf("%s does not carry %s", fx.Name, first.sourceName())
		}
		if _, ok := fx.sources[second.sourceName()]; !ok {
			t.Errorf("%s does not carry %s", fx.Name, second.sourceName())
		}
	}

	// A fixture naming its own root keys a cache of its own, so the bodies of
	// one directory do not reach the tree a second directory compiled.
	apart := &Fixture{Name: "apart", Path: "other/apart.phpt", PHP: "<?php echo 3;"}
	ShareSources([]*Fixture{first, apart})
	if len(apart.sources) != 1 {
		t.Errorf("apart carries %d bodies, want 1", len(apart.sources))
	}
}

// TestFixture_SetAppRoot covers the second filesystem a fixture runs over: the
// application root holding the prelude. The fixture directory answers first,
// so a fixture's own relative include keeps meaning what it meant, and the
// run reads the prelude off the root beneath it.
func TestFixture_SetAppRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prelude.php"), []byte("<?php\nfunction prelude_tag() { return \"tagged\"; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fx, err := ParseFixture([]byte("name: approot\ndescription: application root prelude\n---\n<?php\necho prelude_tag();\n---\ntagged"), filepath.Join(dir, "approot.phpt"))
	if err != nil {
		t.Fatal(err)
	}
	fx.SetAppRoot(dir, "prelude.php")

	res := RunFixture(t.Context(), fx)
	if !res.Passed {
		t.Fatalf("fixture failed: %s", res.FailureReason)
	}

	// The run skips an include the application does not carry: the setting
	// says what to load when the tree holds it.
	fx.SetAppRoot(dir, "prelude.php", "absent.php")
	if res := RunFixture(t.Context(), fx); !res.Passed {
		t.Fatalf("fixture failed with a prelude file that is not there: %s", res.FailureReason)
	}
}

// TestFixture_SetDatabase covers the connections a fixture's Database bindings
// resolve through. The suite passes the provider its setup hook applied the
// schema with, so runnerOptions carries that one into the run and opens no
// second pool over the same DSN.
func TestFixture_SetDatabase(t *testing.T) {
	provider := database.New(nil)

	fx := &Fixture{Name: "db", Path: "db.phpt"}
	if got := fx.runnerOptions().Database; got != nil {
		t.Errorf("Database = %v before SetDatabase, want nothing", got)
	}

	fx.SetDatabase(provider)
	if got := fx.runnerOptions().Database; got != model.DatabaseProvider(provider) {
		t.Errorf("Database = %v, want the provider the suite built", got)
	}
}

// TestFixture_SetMail covers the mail servers a fixture may name. Nothing
// leaves a run of the fixtures: construction is a name lookup, and the names
// are what the fixtures assert on.
func TestFixture_SetMail(t *testing.T) {
	provider := mail.NewProvider(nil)

	fx := &Fixture{Name: "mail", Path: "mail.phpt"}
	if got := fx.runnerOptions().Mail; got != nil {
		t.Errorf("Mail = %v before SetMail, want nothing", got)
	}

	fx.SetMail(provider)
	if got := fx.runnerOptions().Mail; got != model.MailProvider(provider) {
		t.Errorf("Mail = %v, want the provider the suite built", got)
	}
}

// TestFixture_SetCoverage covers the collector the runtime runner reports
// statements to. Only that runner carries one: flatstack has no coverage
// support, and the php runner is a second process.
func TestFixture_SetCoverage(t *testing.T) {
	fx := &Fixture{Name: "cover", Path: "cover.phpt"}
	if got := fx.Coverage(); got != nil {
		t.Errorf("Coverage() = %v before SetCoverage, want nothing", got)
	}

	collector := coverage.New()
	fx.SetCoverage(collector)
	if got := fx.Coverage(); got != collector {
		t.Errorf("Coverage() = %v, want the collector that was installed", got)
	}
}

// TestFixture_RootDir covers the directory a fixture's includes resolve
// against, which is also where the php runner executes. A fixture naming
// `root:` moves it, relative to its own directory, for a tree phpscript does
// not embed.
func TestFixture_RootDir(t *testing.T) {
	for _, tc := range []struct {
		path string
		root string
		want string
	}{
		{"fixtures/area/one.phpt", "", filepath.FromSlash("fixtures/area")},
		{"fixtures/area/one.phpt", "vendor", filepath.FromSlash("fixtures/area/vendor")},
		{"one.phpt", "", "."},
	} {
		fx := &Fixture{Path: filepath.FromSlash(tc.path), Root: tc.root}
		if got := fx.RootDir(); got != tc.want {
			t.Errorf("RootDir() for path %q root %q = %q, want %q", tc.path, tc.root, got, tc.want)
		}
	}
}

// TestParseGolden covers the second fixture form: a _test.php body php can run
// where it lies, and the _test.txt beside it. ParseGolden takes both as they
// are, with CRLF normalised so a file checked out on Windows compares equal,
// and the trailing newlines of the expected output dropped.
func TestParseGolden(t *testing.T) {
	fx := ParseGolden(
		[]byte("<?php\r\necho \"ok\";\r\n"),
		[]byte("ok\r\n\n"),
		filepath.FromSlash("fixtures/area/echo_test.php"),
	)

	if fx.Name != "echo" {
		t.Errorf("Name = %q, want \"echo\"", fx.Name)
	}
	if want := "<?php\necho \"ok\";\n"; fx.PHP != want {
		t.Errorf("PHP = %q, want %q", fx.PHP, want)
	}
	if fx.Expected != "ok" {
		t.Errorf("Expected = %q, want \"ok\"", fx.Expected)
	}
	if want := filepath.FromSlash("fixtures/area/echo_test.php"); fx.Path != want {
		t.Errorf("Path = %q, want %q", fx.Path, want)
	}
}

// TestFindFixtures covers discovery over a real tree: both fixture forms, in
// path order, and a _test.php with no expected output beside it left out. A
// path ending in /... walks the subdirectories, and one without it reads only
// the directory named.
func TestFindFixtures(t *testing.T) {
	dir := writeTree(t,
		"doc.phpt", "name: doc\ndescription: a document\n---\n<?php echo \"ok\";\n---\nok",
		"body_test.php", "<?php echo \"ok\";\n",
		"body_test.txt", "ok\n",
		// A body with no expected output beside it stays a php file in the
		// tree, and no fixture.
		"loose_test.php", "<?php echo \"loose\";\n",
		"nested/deep.phpt", "name: deep\ndescription: a nested document\n---\n<?php echo \"ok\";\n---\nok",
	)

	flat, err := FindFixtures([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixtureNames(flat), []string{"body", "doc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindFixtures(dir) = %v, want %v", got, want)
	}

	deep, err := FindFixtures([]string{filepath.Join(dir, "...")})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixtureNames(deep), []string{"body", "deep", "doc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindFixtures(dir/...) = %v, want %v", got, want)
	}

	one, err := FindFixtures([]string{filepath.Join(dir, "doc.phpt")})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixtureNames(one), []string{"doc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindFixtures(file) = %v, want %v", got, want)
	}
}

// fixtureNames lists the fixture names in discovery order.
func fixtureNames(fixtures []*Fixture) []string {
	names := make([]string, 0, len(fixtures))
	for _, fx := range fixtures {
		names = append(names, fx.Name)
	}
	sort.Strings(names)
	return names
}
