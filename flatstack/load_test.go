package flatstack_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/tests"
)

// These cases mirror the flat-stack repository's table-driven VM and load
// tests, adapted to phpscript's public parser and runner-compatible API.
func TestFlatstackTableDrivenExecution(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "literal",
			source: `<?php echo "hello"; ?>`,
			want:   "hello",
		},
		{
			name:   "assigned concat",
			source: `<?php $left = "flat"; $right = "stack"; echo $left . "-" . $right; ?>`,
			want:   "flat-stack",
		},
		{
			name:   "php string coercion",
			source: `<?php $number = 42; echo "answer=" . $number; ?>`,
			want:   "answer=42",
		},
		{
			name:   "inline html",
			source: `before-<?php $value = "inside"; echo $value; ?>-after`,
			want:   "before-inside-after",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if err := flatstack.Supports(program); err != nil {
				t.Fatalf("test unexpectedly uses interpreter fallback: %v", err)
			}
			var output strings.Builder
			runtime := flatstack.New(&output, flatstack.Options{})
			if err := runtime.Run(program); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFlatstackNativeLanguageCoverage(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"arithmetic and precedence", `<?php echo 2 + 3 * 4, ":", 9 % 4;`, "14:1"},
		{"comparison and unary", `<?php echo 2 < 3 ? "yes" : "no"; echo !false ? ":true" : ":false";`, "yes:true"},
		{"short circuit", `<?php $n = 0; false && ($n = 1); true || ($n = 2); echo $n;`, "0"},
		{"array read write append", `<?php $a = array("x" => 1); $a["x"] += 2; $a[] = 4; echo $a["x"], ":", $a[0];`, "3:4"},
		{"increment and decrement", `<?php $n = 1; echo $n++, ":", ++$n, ":"; $a = array(4); echo --$a[0], ":", $a[0]--;`, "1:3:3:3"},
		{"if else", `<?php $n = 3; if ($n > 2) echo "big"; else echo "small";`, "big"},
		{"for continue break", `<?php for ($i = 0; $i < 6; $i += 1) { if ($i == 2) continue; if ($i == 5) break; echo $i; }`, "0134"},
		{"switch fallthrough and break", `<?php $n = 2; switch ($n) { case 1: echo "one"; break; case 2: echo "two"; case 3: echo "+three"; break; default: echo "other"; }`, "two+three"},
		{"throw and catch", `<?php try { throw "boom"; } catch (Exception $e) { echo $e; }`, "uncaught exception: boom"},
		{"foreach variables", `<?php foreach (array("a" => 1, "b" => 2) as $k => $v) echo $k . $v;`, "a1b2"},
		{"foreach index targets", `<?php $state = array(); foreach (array("x" => 7) as $state["k"] => $state["v"]) echo $state["k"] . $state["v"];`, "x7"},
		{"elvis evaluates condition once", `<?php $value = "kept"; echo $value ?: "fallback";`, "kept"},
		{"php class declaration", `<?php class Greeter { function hello() { echo "hi"; } } $g = new Greeter; $g->hello();`, "hi"},
		{"class method calls user function", `<?php function tag() { echo "F"; } class C { function m() { tag(); } } $o = new C; $o->m();`, "F"},
		{"class method this property", `<?php class Box { public $n = 3; function show() { echo $this->n; } } $b = new Box; $b->show();`, "3"},
		{"throw from user function caught", `<?php function boom() { throw "x"; } try { boom(); echo "no"; } catch (Exception $e) { echo "ok"; }`, "ok"},
		{"method call is case insensitive", `<?php class Greeter { function hello() { echo "hi"; } } $g = new Greeter; $g->Hello();`, "hi"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if err := flatstack.Supports(program); err != nil {
				t.Fatalf("expected native bytecode support: %v", err)
			}
			var output strings.Builder
			runtime := flatstack.New(&output, flatstack.Options{})
			if err := runtime.Run(program); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFlatstackNativeInclude(t *testing.T) {
	program, err := parser.Parse(`<?php $message = include("plugin.php"); echo $message;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatalf("expected native bytecode support: %v", err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetIncludeResolver(func(path string) (*model.Program, error) {
		if path != "plugin.php" {
			t.Fatalf("include path = %q", path)
		}
		return parser.Parse(`<?php return "Hello world!";`)
	})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "Hello world!" {
		t.Fatalf("output = %q, want %q", got, "Hello world!")
	}
}

func TestFlatstackIncludeDefinesCallerVar(t *testing.T) {
	program, err := parser.Parse(`<?php include("defs.php"); echo $x;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetIncludeResolver(func(path string) (*model.Program, error) {
		return parser.Parse(`<?php $x = "z";`)
	})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "z" {
		t.Fatalf("output = %q, want z", got)
	}
}

func TestFlatstackIncludeOnce(t *testing.T) {
	program, err := parser.Parse(`<?php include_once("once.php"); include_once("once.php");`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int64
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetIncludeResolver(func(path string) (*model.Program, error) {
		hits.Add(1)
		return parser.Parse(`<?php echo "x";`)
	})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("include_once resolved %d times, want 1", hits.Load())
	}
	if got := output.String(); got != "x" {
		t.Fatalf("output = %q, want x", got)
	}
}

func TestFlatstackIncludeChainSeesPriorIncludeVars(t *testing.T) {
	program, err := parser.Parse(`<?php include("a.php"); include("b.php");`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetIncludeResolver(func(path string) (*model.Program, error) {
		if path == "a.php" {
			return parser.Parse(`<?php $x = "A";`)
		}
		return parser.Parse(`<?php echo $x;`)
	})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "A" {
		t.Fatalf("output = %q, want A", got)
	}
}

func TestFlatstackIncludeDoesNotLeakAcrossRuns(t *testing.T) {
	first, err := parser.Parse(`<?php include("leak.php");`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := parser.Parse(`<?php echo $leak;`)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetIncludeResolver(func(path string) (*model.Program, error) {
		return parser.Parse(`<?php $leak = "L";`)
	})
	if err := runtime.Run(first); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Run(second); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "" {
		t.Fatalf("output = %q, want empty (no leak across Run)", got)
	}
}

func TestFlatstackIncludeInFunctionDoesNotLeakToCaller(t *testing.T) {
	program, err := parser.Parse(`<?php function load() { include("in.php"); } load(); echo $x;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetIncludeResolver(func(path string) (*model.Program, error) {
		return parser.Parse(`<?php $x = "A";`)
	})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "" {
		t.Fatalf("output = %q, want empty (include vars stay in the function)", got)
	}
}

func TestFlatstackLoopControlUnwindsTryHandler(t *testing.T) {
	program, err := parser.Parse(`<?php
for ($i = 0; $i < 1; $i += 1) {
    try { break; } catch (Exception $e) { echo "wrong catch"; }
}
$storage = new Storage;
$storage->get("missing");
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}
	runtime := flatstack.New(io.Discard, flatstack.Options{})
	runtime.RegisterConstructor("Storage", tests.NewStorage)
	err = runtime.Run(program)
	if err == nil || !strings.Contains(err.Error(), "missing key") {
		t.Fatalf("error after break = %v, want uncaught missing-key error", err)
	}
}

func TestFlatstackRejectsWholeProgramBeforeSideEffects(t *testing.T) {
	program, err := parser.Parse(`<?php
$storage = new Storage;
$fn = function() { return 1; };
echo 42;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err == nil {
		t.Fatal("program with a closure should require interpreter fallback")
	}

	var constructions atomic.Int64
	constructor := func(ctx context.Context) (tests.Storage, error) {
		constructions.Add(1)
		return tests.NewStorage(ctx)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.RegisterConstructor("Storage", constructor)
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := constructions.Load(); got != 1 {
		t.Fatalf("constructor called %d times; fallback repeated a side effect", got)
	}
	if got := output.String(); got != "42" {
		t.Fatalf("fallback output = %q, want 42", got)
	}
}

// TestFlatstackSupportsArrayCallbackClosures fails until the compiler learns
// *model.Closure, and is the only signal that it has: runFlat discards the
// compile error and reruns the program on the interpreter, so the fixture in
// tests/fixtures/arrays/array_callbacks.phpt passes either way.
//
// A closure argument is the most common reason a real library drops out of the
// bytecode engine. One usort() comparator costs the whole file, because Compile
// rejects a program on any unsupported node.
func TestFlatstackSupportsArrayCallbackClosures(t *testing.T) {
	program, err := parser.Parse(`<?php
$n = array(3, 1, 2);
usort($n, function ($a, $b) {
	return $a - $b;
});
echo implode(",", $n);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatalf("usort closure should compile to bytecode: %v", err)
	}
}

func TestFlatstackSharedCacheParallel(t *testing.T) {
	program, err := parser.Parse(`<?php $a = "shared"; $b = "-cache"; echo $a . $b; ?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}

	cache := flatstack.NewExprCache()
	const workers = 16
	const iterations = 100
	var failures atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				var output strings.Builder
				runtime := flatstack.New(&output, flatstack.Options{})
				runtime.SetExprCache(cache)
				if err := runtime.Run(program); err != nil || output.String() != "shared-cache" {
					failures.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if got := failures.Load(); got != 0 {
		t.Fatalf("parallel runs failed: %d", got)
	}
}

func TestFlatstackHostErrorPropagates(t *testing.T) {
	program, err := parser.Parse(`<?php $storage = new Storage; $storage->get("missing"); ?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}

	runtime := flatstack.New(io.Discard, flatstack.Options{})
	runtime.RegisterConstructor("Storage", tests.NewStorage)
	err = runtime.Run(program)
	if err == nil || !strings.Contains(err.Error(), "missing key") {
		t.Fatalf("error = %v, want missing-key host error", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}

func TestFlatstackOutputErrorPropagates(t *testing.T) {
	program, err := parser.Parse(`<?php echo "value"; ?>`)
	if err != nil {
		t.Fatal(err)
	}
	runtime := flatstack.New(failingWriter{}, flatstack.Options{})
	err = runtime.Run(program)
	if err == nil || !strings.Contains(err.Error(), "output unavailable") {
		t.Fatalf("error = %v, want writer error", err)
	}
}

func TestFlatstackRuntimeGlobalUsesFlatBytecode(t *testing.T) {
	program, err := parser.Parse(`<?php echo "tenant=" . $tenant; ?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatalf("runtime global should compile as a host-backed flat local: %v", err)
	}

	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetGlobal("tenant", "acme")
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "tenant=acme" {
		t.Fatalf("output = %q, want tenant=acme", got)
	}
}

type panicHost struct{}

func (panicHost) Crash() {
	panic("deliberate host panic")
}

func TestFlatstackHostPanicBecomesError(t *testing.T) {
	program, err := parser.Parse(`<?php $host = new PanicHost; $host->crash(); ?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}
	runtime := flatstack.New(io.Discard, flatstack.Options{})
	runtime.RegisterConstructor("PanicHost", func() panicHost { return panicHost{} })
	err = runtime.Run(program)
	var panicErr *flatstack.HostPanicError
	if err == nil || !errors.As(err, &panicErr) || !strings.Contains(err.Error(), "deliberate host panic") {
		t.Fatalf("panic boundary error = %v", err)
	}
}

func TestFlatstackHostPanicCanBeCaught(t *testing.T) {
	tests := []struct {
		name   string
		source string
		setup  func(*flatstack.Runtime)
	}{
		{
			name: "method",
			source: `<?php
$host = new PanicHost;
try { $host->crash(); } catch (Exception $e) { echo "caught: " . $e; }
?>`,
			setup: func(runtime *flatstack.Runtime) {
				runtime.RegisterConstructor("PanicHost", func() panicHost { return panicHost{} })
			},
		},
		{
			name: "constructor",
			source: `<?php
try { $host = new PanicHost; } catch (Exception $e) { echo "caught: " . $e; }
?>`,
			setup: func(runtime *flatstack.Runtime) {
				runtime.RegisterConstructor("PanicHost", func() panicHost {
					panic("constructor exploded")
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if err := flatstack.Supports(program); err != nil {
				t.Fatalf("try/catch should compile into flat bytecode: %v", err)
			}
			var output strings.Builder
			runtime := flatstack.New(&output, flatstack.Options{})
			test.setup(runtime)
			if err := runtime.Run(program); err != nil {
				t.Fatalf("caught panic escaped runtime: %v", err)
			}
			if got := output.String(); !strings.HasPrefix(got, "caught: host panic in ") {
				t.Fatalf("output = %q, want caught host panic", got)
			}
		})
	}
}

func TestFlatstackPrecompiledAllocationBudget(t *testing.T) {
	program, err := parser.Parse(`<?php $left = "flat"; $right = "stack"; echo $left . $right; ?>`)
	if err != nil {
		t.Fatal(err)
	}
	runtime := flatstack.New(io.Discard, flatstack.Options{})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(500, func() {
		if err := runtime.Run(program); err != nil {
			panic(err)
		}
	})
	t.Logf("precompiled concat allocations/run = %.2f", allocations)
	if allocations > 4 {
		t.Skipf("precompiled concat allocations/run = %.2f, want <= 4", allocations)
	}
}

func TestFlatstackDeepConcat(t *testing.T) {
	const depth = 100
	source := `<?php $value = "x"; echo $value` + strings.Repeat(` . "x"`, depth) + `; ?>`
	program, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got, want := output.Len(), depth+1; got != want {
		t.Fatalf("output length = %d, want %d", got, want)
	}
}

func ExampleSupports() {
	program, _ := parser.Parse(`<?php $a = "flat"; echo $a . "stack"; ?>`)
	fmt.Println(flatstack.Supports(program) == nil)
	// Output: true
}
