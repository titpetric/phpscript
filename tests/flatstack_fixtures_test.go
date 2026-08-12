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

// TestFlatstackFixtures runs opted-in fixtures through flat bytecode.
func TestFlatstackFixtures(t *testing.T) {
	entries, err := fs.ReadDir(fixturesFS, "fixtures")
	if err != nil {
		t.Fatal(err)
	}
	selected := 0
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
		if !fixture.Flatstack {
			continue
		}
		selected++
		t.Run(fixture.Name, func(t *testing.T) {
			program, err := parser.Parse(fixture.PHP)
			if err != nil {
				t.Fatal(err)
			}
			if err := flatstack.Supports(program); err != nil {
				t.Fatalf("fixture marked flatstack is unsupported: %v", err)
			}
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
	if selected == 0 {
		t.Fatal("no flatstack fixtures selected")
	}
}
