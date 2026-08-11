package tests

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
)

var flatIncludeCache = flatstack.NewIncludeCache()
var flatExprCache = flatstack.NewExprCache()

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

func runFlatstackFixture(ctx context.Context, fixture fixture) (string, error) {
	program, err := parser.Parse(fixture.PHP)
	if err != nil {
		return "Internal Server Error", err
	}
	var output strings.Builder
	runtime := newFlatstackTestRuntime(&output, ctx, strings.NewReader(fixture.Stdin))
	if err := runtime.Run(program); err != nil {
		if _, ok := flatstack.IsExit(err); ok {
			return output.String(), nil
		}
		return "Internal Server Error", err
	}
	return output.String(), nil
}

// TestFlatstackImportSwapFixtures runs the end-to-end fixture contract through
// the flatstack import. Currently supported ASTs use bytecode and the remainder
// exercise the compatibility fallback.
func TestFlatstackImportSwapFixtures(t *testing.T) {
	entries, err := fs.ReadDir(fixturesFS, "fixtures")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".phpt") {
			continue
		}
		data, err := fixturesFS.ReadFile("fixtures/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		fixture, err := parseFixture(data)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fixture.Name, func(t *testing.T) {
			output, runErr := runFlatstackFixture(t.Context(), fixture)
			if fixture.Error != "" {
				if runErr == nil || !errorChainContains(runErr, fixture.Error) {
					t.Fatalf("error = %v, want containing %q", runErr, fixture.Error)
				}
			} else if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			got := strings.TrimRight(output, "\n")
			want := strings.TrimRight(fixture.Expected, "\n")
			if got != want {
				t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

func TestFlatstackNativeFixtureCoverage(t *testing.T) {
	expectedNative := map[string]bool{
		"array_indexing.phpt":                   true,
		"autoloading_default.phpt":              true,
		"autoloading_missing.phpt":              true,
		"condition_syntax.phpt":                 true,
		"die_exit.phpt":                         true,
		"exception.phpt":                        true,
		"exception_response_code.phpt":          true,
		"php_array_splice.phpt":                 true,
		"php_foreach_syntax.phpt":               true,
		"stdin.phpt":                            true,
		"storage_constructor_error.phpt":        true,
		"storage_constructor_error_caught.phpt": true,
		"storage_context.phpt":                  true,
		"storage_lifecycle.phpt":                true,
		"storage_list.phpt":                     true,
		"storage_method_error.phpt":             true,
		"storage_method_error_caught.phpt":      true,
	}
	entries, err := fs.ReadDir(fixturesFS, "fixtures")
	if err != nil {
		t.Fatal(err)
	}
	native, total := 0, 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".phpt") {
			continue
		}
		total++
		data, err := fixturesFS.ReadFile("fixtures/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		fixture, err := parseFixture(data)
		if err != nil {
			t.Fatal(err)
		}
		program, err := parser.Parse(fixture.PHP)
		if err != nil {
			t.Fatal(err)
		}
		isNative := flatstack.Supports(program) == nil
		if isNative {
			native++
		}
		if isNative != expectedNative[entry.Name()] {
			t.Errorf("%s native = %v, want %v", entry.Name(), isNative, expectedNative[entry.Name()])
		}
	}
	if native != len(expectedNative) || total != 28 {
		t.Fatalf("native=%d fallback=%d total=%d; want native=%d total=28", native, total-native, total, len(expectedNative))
	}
	t.Logf("flatstack fixture coverage: native=%d fallback=%d total=%d", native, total-native, total)
}
