package runner_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/telemetry"
)

// isolationTree is the source root the pooled runtimes serve: one include
// target and one directory for chdir to move into.
var isolationTree = fstest.MapFS{
	"inc.php":      {Data: []byte("<?php\nfunction inc_fn() { return \"inc\"; }\n")},
	"sub/dir.keep": {Data: []byte("")},
}

// stateRecorder records the last lifecycle state published to it, so a test
// can see what a pooled runtime announces between requests.
type stateRecorder struct {
	state telemetry.State
}

func (r *stateRecorder) UpdateStatus(_ context.Context, state telemetry.State) { r.state = state }

func (r *stateRecorder) Trace(context.Context, string, ...telemetry.Kind) *telemetry.Span {
	return nil
}

// runPooled runs one source string on rt the way a pooled request does: the
// output pushed over the runtime for the length of the run.
func runPooled(t *testing.T, rt *runner.Runtime, src string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rt.PushOutput(&out)
	prog, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = rt.Run(prog)
	return out.String(), err
}

// TestSessionPoolIsolation runs two programs through one pooled runtime and
// asserts the second sees nothing of the first: one case per kind of state a
// script can set, each written so that a reset that stops clearing that state
// fails the case. The pool holds one runtime, and each case asserts it got the
// same one back, so the isolation under test is the reset's and never a second
// runtime's.
func TestSessionPoolIsolation(t *testing.T) {
	recorder := &stateRecorder{}
	pool := runner.NewSessionPool(1, func() *runner.Runtime {
		rt := runner.New(io.Discard, runner.Options{RootFS: isolationTree})
		rt.Observe(recorder)
		stdlib.Register(rt)
		rt.FreezeStdlib()
		return rt
	})

	cases := []struct {
		name string
		// first is the program that writes the state under test. firstErr
		// says its error is the point; firstWant, when set, pins its output.
		first     string
		firstErr  bool
		firstWant string
		// second probes for the state and echoes what it found; between is a
		// Go-level probe run after the reset and before the second program.
		second  string
		want    string
		between func(t *testing.T, rt *runner.Runtime)
	}{
		{
			name:   "global",
			first:  `<?php $leak = "secret";`,
			second: `<?php echo isset($leak) ? "leak" : "clean";`,
			want:   "clean",
		},
		{
			name:   "constant via define",
			first:  `<?php define("POOL_LEAK", 1);`,
			second: `<?php echo defined("POOL_LEAK") ? "leak" : "clean";`,
			want:   "clean",
		},
		{
			name:   "user function",
			first:  `<?php function pool_leak_fn() {}`,
			second: `<?php echo function_exists("pool_leak_fn") ? "leak" : "clean";`,
			want:   "clean",
		},
		{
			name:   "class",
			first:  `<?php class PoolLeak {}`,
			second: `<?php echo class_exists("PoolLeak") ? "leak" : "clean";`,
			want:   "clean",
		},
		{
			name:   "interface",
			first:  `<?php interface PoolLeakContract {}`,
			second: `<?php echo "clean";`,
			want:   "clean",
			between: func(t *testing.T, rt *runner.Runtime) {
				if declared, _ := rt.InterfaceExists("PoolLeakContract", false); declared {
					t.Error("the previous request's interface survived the reset")
				}
			},
		},
		{
			name:   "static property",
			first:  `<?php class Holder { public static $n = 0; } Holder::$n = 9;`,
			second: `<?php class Holder { public static $n = 0; } echo Holder::$n;`,
			want:   "0",
		},
		{
			name:      "class constant cache",
			first:     `<?php class Version { const N = 1; } echo Version::N;`,
			firstWant: "1",
			second:    `<?php class Version { const N = 2; } echo Version::N;`,
			want:      "2",
		},
		{
			name:   "function static variable",
			first:  `<?php function counter() { static $n = 0; $n++; return $n; } counter(); counter();`,
			second: `<?php function counter() { static $n = 0; $n++; return $n; } echo counter();`,
			want:   "1",
		},
		{
			name:   "autoloader",
			first:  `<?php spl_autoload_register(function ($name) { echo "autoload:", $name; });`,
			second: `<?php echo class_exists("PoolMissing", true) ? "leak" : "clean";`,
			want:   "clean",
		},
		{
			name:   "shutdown callback",
			first:  `<?php register_shutdown_function(function () { echo "shutdown"; });`,
			second: `<?php echo "clean";`,
			want:   "clean",
		},
		{
			name:   "output buffer left open",
			first:  `<?php ob_start(); echo "hidden";`,
			second: `<?php echo ob_get_level(); echo ob_get_contents() === false ? "clean" : "leak";`,
			want:   "0clean",
		},
		{
			name:   "ignore_user_abort",
			first:  `<?php ignore_user_abort(true);`,
			second: `<?php echo "clean";`,
			want:   "clean",
			between: func(t *testing.T, rt *runner.Runtime) {
				if rt.IgnoreUserAbort() {
					t.Error("ignore_user_abort(true) survived the reset")
				}
			},
		},
		{
			name:   "set_time_limit",
			first:  `<?php set_time_limit(30);`,
			second: `<?php echo "clean";`,
			want:   "clean",
			between: func(t *testing.T, rt *runner.Runtime) {
				if limit := rt.TimeLimit(); limit != 0 {
					t.Errorf("time limit %v survived the reset", limit)
				}
			},
		},
		{
			name:   "superglobal entry",
			first:  `<?php $_GET["leak"] = "x"; $_SERVER["LEAK"] = "y";`,
			second: `<?php echo isset($_GET["leak"]) || isset($_SERVER["LEAK"]) ? "leak" : "clean";`,
			want:   "clean",
		},
		{
			name:     "uncaught error status",
			first:    `<?php pool_undefined_function();`,
			firstErr: true,
			second:   `<?php echo "clean";`,
			want:     "clean",
			between: func(t *testing.T, rt *runner.Runtime) {
				// What a pooled runtime announces at the start of the next
				// request is whatever SetContext publishes, so the probe is
				// the same call the handler makes.
				rt.SetContext(context.Background())
				if recorder.state == telemetry.StateError {
					t.Error("the previous request's error state survived the reset")
				}
			},
		},
		{
			name:      "included file",
			first:     `<?php include_once "inc.php"; echo inc_fn();`,
			firstWant: "inc",
			second:    `<?php include_once "inc.php"; echo function_exists("inc_fn") ? inc_fn() : "missing";`,
			want:      "inc",
		},
		{
			name:   "putenv",
			first:  `<?php putenv("POOL_LEAK_ENV=x");`,
			second: `<?php echo getenv("POOL_LEAK_ENV") === false ? "clean" : "leak";`,
			want:   "clean",
		},
		{
			name:   "chdir",
			first:  `<?php chdir("sub");`,
			second: `<?php echo getcwd();`,
			want:   "/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := pool.Get()
			out, err := runPooled(t, first, tc.first)
			if tc.firstErr != (err != nil) {
				t.Fatalf("first program error = %v, expected an error: %v", err, tc.firstErr)
			}
			if tc.firstWant != "" && out != tc.firstWant {
				t.Fatalf("first program output = %q, want %q", out, tc.firstWant)
			}
			pool.Put(first)

			second := pool.Get()
			defer pool.Put(second)
			if second != first {
				t.Fatal("the pool built a second runtime, so the reuse under test never happened")
			}
			if tc.between != nil {
				tc.between(t, second)
			}
			got, err := runPooled(t, second, tc.second)
			if err != nil {
				t.Fatalf("second program: %v", err)
			}
			if got != tc.want {
				t.Fatalf("second program output = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSessionPoolBounds covers the free list's contract: an empty pool builds,
// Put keeps a runtime for the next Get, and the list never holds more than its
// size.
func TestSessionPoolBounds(t *testing.T) {
	var built int
	pool := runner.NewSessionPool(1, func() *runner.Runtime {
		built++
		return runner.New(io.Discard, runner.Options{})
	})

	first, second := pool.Get(), pool.Get()
	if built != 2 {
		t.Fatalf("two Gets on an empty pool built %d runtimes, want 2", built)
	}
	pool.Put(first)
	pool.Put(second) // over the bound of one, dropped
	if got := pool.Get(); got != first {
		t.Fatal("Get did not return the runtime Put kept")
	}
	if pool.Get() == second {
		t.Fatal("a Put over the pool's size kept the runtime it should drop")
	}
	if built != 3 {
		t.Fatalf("built %d runtimes, want 3: the second Put was over the bound", built)
	}
}
