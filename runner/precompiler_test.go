package runner_test

import (
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
)

var precompileFS = fstest.MapFS{
	"public/index.php": {Data: []byte(`<?php echo "home";`)},
	"lib/greet.php":    {Data: []byte(`<?php function greet() { return "greeted"; }`)},
	"lib/broken.php":   {Data: []byte(`<?php function ( { `)},
	"public/style.css": {Data: []byte(`body { color: red; }`)},
}

// TestPrecompiler covers the walk: every .php file that parses is cached under
// the path a script would name it by, and a file that does not parse is left
// out instead of failing the pass.
func TestPrecompiler(t *testing.T) {
	includes := runner.NewIncludeCache()
	exprs := runner.NewExprCache()

	cached := runner.Precompiler{Root: precompileFS, Includes: includes, Exprs: exprs}.Run()
	if cached != 2 {
		t.Fatalf("cached = %d, want 2", cached)
	}
	if includes.Len() != 2 {
		t.Fatalf("include cache holds %d, want 2", includes.Len())
	}
	for _, name := range []string{"public/index.php", "lib/greet.php"} {
		if _, ok := includes.Get(name); !ok {
			t.Fatalf("%s was not cached", name)
		}
	}
	if _, ok := includes.Get("lib/broken.php"); ok {
		t.Fatal("a file that does not parse was cached")
	}
}

// TestPrecompilerEmptyTree covers the inputs that have nothing to walk: a
// Precompiler is constructed by a host that may not have configured one.
func TestPrecompilerEmptyTree(t *testing.T) {
	includes := runner.NewIncludeCache()
	for name, p := range map[string]runner.Precompiler{
		"no root":    {Includes: includes},
		"no cache":   {Root: precompileFS},
		"no sources": {Root: fstest.MapFS{"README.md": {Data: []byte("nothing to parse")}}, Includes: includes},
	} {
		if cached := p.Run(); cached != 0 {
			t.Fatalf("%s: cached = %d, want 0", name, cached)
		}
	}
}

// TestPrecompilerServesLoadFile covers what the walk buys a request: an
// entrypoint LoadFile answers with the program the walk parsed, rather than
// with a second parse of the same bytes.
func TestPrecompilerServesLoadFile(t *testing.T) {
	includes := runner.NewIncludeCache()
	runner.Precompiler{Root: precompileFS, Includes: includes, Workers: 1}.Run()

	precompiled, _ := includes.Get("public/index.php")

	rt := runner.New(io.Discard, runner.Options{RootFS: precompileFS, Precompile: true})
	rt.SetIncludeCache(includes)
	program, err := rt.LoadFile("public/index.php")
	if err != nil {
		t.Fatal(err)
	}
	if program != precompiled {
		t.Fatal("LoadFile parsed the entrypoint again instead of reading the precompiled program")
	}
}

// TestPrecompilerOffReparses covers the other half of the option: with it off,
// LoadFile reads and parses the file every time, which is what a CLI run and an
// application editing its own sources expect.
func TestPrecompilerOffReparses(t *testing.T) {
	for _, precompile := range []bool{false, true} {
		rt := runner.New(io.Discard, runner.Options{RootFS: precompileFS, Precompile: precompile})
		first, err := rt.LoadFile("public/index.php")
		if err != nil {
			t.Fatal(err)
		}
		second, err := rt.LoadFile("public/index.php")
		if err != nil {
			t.Fatal(err)
		}
		if (first == second) != precompile {
			t.Fatalf("precompile=%v: reused the parsed program = %v", precompile, first == second)
		}
	}
}

// TestPrecompilerBrokenEntrypointStillFails covers the error a precompiled tree
// must keep reporting: the file was left out of the cache, so the request that
// reaches it parses it and fails the way it does without precompilation.
func TestPrecompilerBrokenEntrypointStillFails(t *testing.T) {
	includes := runner.NewIncludeCache()
	runner.Precompiler{Root: precompileFS, Includes: includes}.Run()

	rt := runner.New(io.Discard, runner.Options{RootFS: precompileFS, Precompile: true})
	rt.SetIncludeCache(includes)
	if _, err := rt.LoadFile("lib/broken.php"); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("err = %v, want a parse error", err)
	}
}
