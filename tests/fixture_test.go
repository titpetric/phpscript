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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v3"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
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

	type result struct {
		area    string
		name    string
		passed  bool
		skipped bool
	}
	var results []result

	for _, area := range areas {
		t.Run(area.Name, func(t *testing.T) {
			for _, fx := range area.Fixtures {
				// t.Run reports a skipped subtest as passing, so the skip is
				// recorded here rather than read back from it: a fixture the
				// host could not run must not be summarised as one that ran.
				skipped := false
				passed := t.Run(fx.Name, func(t *testing.T) {
					res := RunFixture(t.Context(), fx)
					// A fixture the host cannot run at all, a Go plugin in a
					// build without cgo, is skipped the way a missing php
					// binary is: the answer is about the host, not the fixture.
					if res.Skipped {
						skipped = true
						t.Skip(res.FailureReason)
					}
					if !res.Passed {
						t.Fatalf("fixture failed: %s", res.FailureReason)
					}
				})
				results = append(results, result{area.Name, fx.Name, passed, skipped})
			}
		})
	}

	// Summary table.
	t.Log("Fixture summary:")
	for _, r := range results {
		status := "PASS"
		switch {
		case r.skipped:
			status = "SKIP"
		case !r.passed:
			status = "FAIL"
		}
		t.Logf("  [%s] %s/%s", status, r.area, r.name)
	}
}

var bindingBenchmarkSink int

// BenchmarkGoBindingHTTP compares the same constructor and API calls made by a
// native Go HTTP handler and by a per-request PHP runtime. Source parsing is
// intentionally outside the timed region, and one prebuilt request is reused.
// Per-iteration recorder allocation, runtime setup, reflective binding
// dispatch, execution, and response writing are included where applicable.
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
