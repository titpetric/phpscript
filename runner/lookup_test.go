package runner_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// lookupTree is the source root the resolution tests look symbols up in. Two
// namespaces declare a function of the same short name, which is the case the
// trailing-segment match has to disambiguate rather than guess at.
var lookupTree = fstest.MapFS{
	"handlers/users.php": {Data: []byte(`<?php
namespace App\Users;

function handle($id) {
	return "users:" . $id;
}

function only_here() {
	return "unique";
}
`)},
	"handlers/orders.php": {Data: []byte(`<?php
namespace App\Orders;

function handle($id) {
	return "orders:" . $id;
}
`)},
	"lib/math.php": {Data: []byte(`<?php
function double($n) {
	return $n * 2;
}
`)},
}

// lookupRuntime builds a runtime over files with the standard library on it,
// the shape a host embedding phpscript has.
func lookupRuntime(t testing.TB, out io.Writer, files fstest.MapFS) *runner.Runtime {
	t.Helper()
	if out == nil {
		out = io.Discard
	}
	rt := runner.New(out, runner.Options{RootFS: files})
	stdlib.Register(rt)
	return rt
}

// TestLookupResolvesQualifiedName pins the resolution order: a name spelled in
// full wins, a bare one is matched by its trailing segment, and a partial
// namespace picks between two that share the segment.
func TestLookupResolvesQualifiedName(t *testing.T) {
	tests := []struct {
		symbol string
		want   string
	}{
		{`App\Users\handle`, "users:7"},
		{`Users\handle`, "users:7"},
		{`App\Orders\handle`, "orders:7"},
		{`Orders\handle`, "orders:7"},
		{`\App\Users\handle`, "users:7"},
		{`app\users\HANDLE`, "users:7"},
		{"only_here", "unique"},
	}

	for _, test := range tests {
		t.Run(test.symbol, func(t *testing.T) {
			rt := lookupRuntime(t, nil, lookupTree)
			fn, err := runner.Lookup[func(int) string](rt, test.symbol)
			if err != nil {
				t.Fatalf("Lookup(%q): %v", test.symbol, err)
			}
			if got := fn(7); got != test.want {
				t.Errorf("%s(7) = %q, want %q", test.symbol, got, test.want)
			}
		})
	}
}

// TestLookupReportsAmbiguity holds the second half of the issue's contract: a
// name that reaches more than one declaration is an error naming them, not a
// pick.
func TestLookupReportsAmbiguity(t *testing.T) {
	rt := lookupRuntime(t, nil, lookupTree)
	_, err := runner.Lookup[func(int) string](rt, "handle")

	var lookupErr *runner.LookupError
	if !errors.As(err, &lookupErr) {
		t.Fatalf("Lookup(handle) error = %v, want *runner.LookupError", err)
	}
	want := []string{`App\Orders\handle`, `App\Users\handle`}
	if strings.Join(lookupErr.Candidates, ",") != strings.Join(want, ",") {
		t.Errorf("candidates = %v, want %v", lookupErr.Candidates, want)
	}
}

// TestLookupReportsMissingSymbol keeps "no such function" distinguishable from
// "say which one": the candidate list is what separates them.
func TestLookupReportsMissingSymbol(t *testing.T) {
	rt := lookupRuntime(t, nil, lookupTree)
	_, err := runner.Lookup[func()](rt, "nothing_declares_this")

	var lookupErr *runner.LookupError
	if !errors.As(err, &lookupErr) {
		t.Fatalf("error = %v, want *runner.LookupError", err)
	}
	if len(lookupErr.Candidates) != 0 {
		t.Errorf("candidates = %v, want none", lookupErr.Candidates)
	}
}

// TestLookupRejectsSignature checks the shape at the bind, not at the call, so
// a host wiring a handler wrong hears about it at startup.
func TestLookupRejectsSignature(t *testing.T) {
	t.Run("not a function", func(t *testing.T) {
		rt := lookupRuntime(t, nil, lookupTree)
		if _, err := runner.Lookup[string](rt, "double"); err == nil {
			t.Fatal("Lookup into a string succeeded")
		}
	})

	t.Run("second result is not error", func(t *testing.T) {
		rt := lookupRuntime(t, nil, lookupTree)
		if _, err := runner.Lookup[func() (int, string)](rt, "double"); err == nil {
			t.Fatal("Lookup into (int, string) succeeded")
		}
	})

	t.Run("three results", func(t *testing.T) {
		rt := lookupRuntime(t, nil, lookupTree)
		if _, err := runner.Lookup[func() (int, int, error)](rt, "double"); err == nil {
			t.Fatal("Lookup into (int, int, error) succeeded")
		}
	})
}

// TestLookupConvertsValues pins both edges of the bridge: what a Go argument
// becomes in the scope, and what a PHP return becomes in the result slot.
func TestLookupConvertsValues(t *testing.T) {
	files := fstest.MapFS{"convert.php": {Data: []byte(`<?php
function add($a, $b) { return $a + $b; }
function describe($v) { return gettype($v); }
function flag() { return 1; }
function numeric_string() { return "42"; }
function ratio() { return 1.5; }
function names() { return array("alice", "bob"); }
function ages() { return array("alice" => 30, "bob" => 41); }
function nothing() { return null; }
function spread() { return implode("+", func_get_args()); }
`)}}

	t.Run("int argument widens", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)
		add, err := runner.Lookup[func(int, int) int](rt, "add")
		if err != nil {
			t.Fatal(err)
		}
		if got := add(20, 22); got != 42 {
			t.Errorf("add(20, 22) = %d, want 42", got)
		}
	})

	t.Run("float argument widens", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)
		describe, err := runner.Lookup[func(float32) string](rt, "describe")
		if err != nil {
			t.Fatal(err)
		}
		if got := describe(1.5); got != "double" {
			t.Errorf("describe(float32) = %q, want %q", got, "double")
		}
	})

	t.Run("results take PHP coercions", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)

		flag, err := runner.Lookup[func() bool](rt, "flag")
		if err != nil {
			t.Fatal(err)
		}
		if !flag() {
			t.Error("flag() = false, want true: 1 is truthy")
		}

		count, err := runner.Lookup[func() int](rt, "numeric_string")
		if err != nil {
			t.Fatal(err)
		}
		if got := count(); got != 42 {
			t.Errorf(`numeric_string() = %d, want 42`, got)
		}

		text, err := runner.Lookup[func() string](rt, "ratio")
		if err != nil {
			t.Fatal(err)
		}
		if got := text(); got != "1.5" {
			t.Errorf("ratio() as string = %q, want %q", got, "1.5")
		}
	})

	t.Run("arrays fill slices and maps", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)

		names, err := runner.Lookup[func() []string](rt, "names")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(names(), ","); got != "alice,bob" {
			t.Errorf("names() = %q, want %q", got, "alice,bob")
		}

		ages, err := runner.Lookup[func() map[string]int](rt, "ages")
		if err != nil {
			t.Fatal(err)
		}
		if got := ages(); got["alice"] != 30 || got["bob"] != 41 {
			t.Errorf("ages() = %v, want alice:30 bob:41", got)
		}
	})

	t.Run("null fills any as nil", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)
		nothing, err := runner.Lookup[func() any](rt, "nothing")
		if err != nil {
			t.Fatal(err)
		}
		if got := nothing(); got != nil {
			t.Errorf("nothing() = %v, want nil", got)
		}
	})

	t.Run("a variadic tail is spread", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)
		spread, err := runner.Lookup[func(...string) string](rt, "spread")
		if err != nil {
			t.Fatal(err)
		}
		if got := spread("a", "b", "c"); got != "a+b+c" {
			t.Errorf("func_get_args() = %q, want %q", got, "a+b+c")
		}
	})
}

// TestLookupPassesRequest is the issue's worked example: the *http.Request the
// Go caller was serving is the script's HTTP\Request, so the method, the
// headers, the query and the body all read off the value itself rather than off
// superglobals nothing seeded.
func TestLookupPassesRequest(t *testing.T) {
	files := fstest.MapFS{"handler.php": {Data: []byte(`<?php
namespace App\Handler;

function main(\HTTP\Request $r) {
	$r->parse_form();
	return implode("|", array(
		$r->method,
		$r->url->path,
		$r->url->rawquery,
		$r->header->get("X-Test"),
		$r->user_agent(),
		$r->post_form_value("name"),
	));
}
`)}}

	rt := lookupRuntime(t, nil, files)
	describe, err := runner.Lookup[func(*http.Request) string](rt, "main")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/users?tab=profile", strings.NewReader("name=alice"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-Test", "passed")
	request.Header.Set("User-Agent", "phpscript-test")

	want := "POST|/users|tab=profile|passed|phpscript-test|alice"
	if got := describe(request); got != want {
		t.Errorf("main(request) =\n %q\nwant\n %q", got, want)
	}
}

// TestLookupBindsAnHTTPHandler is the shape the issue is after: a PHP function
// mounted on a Go mux as an http.HandlerFunc, with both halves of the exchange
// passed in as the net/http values they are.
func TestLookupBindsAnHTTPHandler(t *testing.T) {
	files := fstest.MapFS{"handler.php": {Data: []byte(`<?php
namespace App\Handler;

function index(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "text/plain");
	$w->write("hello from " . $r->url->path);
}
`)}}

	rt := lookupRuntime(t, nil, files)
	index, err := runner.Lookup[http.HandlerFunc](rt, `App\Handler\index`)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /greet", index)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/greet", nil))

	if got := response.Body.String(); got != "hello from /greet" {
		t.Errorf("body = %q, want %q", got, "hello from /greet")
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type = %q, want %q", got, "text/plain")
	}
}

// TestLookupCarriesOnlyItsArguments holds the lightweight contract: no request
// was decoded, so the superglobals are absent rather than empty.
func TestLookupCarriesOnlyItsArguments(t *testing.T) {
	files := fstest.MapFS{"scope.php": {Data: []byte(`<?php
function inspect($given) {
	return implode(",", array(
		"given=" . $given,
		"get=" . var_export(isset($_GET), true),
		"post=" . var_export(isset($_POST), true),
		"locals=" . implode("+", array_keys(get_defined_vars())),
	));
}
`)}}

	rt := lookupRuntime(t, nil, files)
	inspect, err := runner.Lookup[func(string) string](rt, "inspect")
	if err != nil {
		t.Fatal(err)
	}

	want := "given=x,get=false,post=false,locals=given"
	if got := inspect("x"); got != want {
		t.Errorf("inspect(x) = %q, want %q", got, want)
	}
}

// TestLookupEchoesToTheRuntimeWriter keeps output where the runtime's output
// goes. A lookup changes how a function is reached, not where it prints.
func TestLookupEchoesToTheRuntimeWriter(t *testing.T) {
	files := fstest.MapFS{"out.php": {Data: []byte(`<?php
function greet($name) { echo "hello ", $name; }
`)}}

	var out strings.Builder
	rt := lookupRuntime(t, &out, files)
	greet, err := runner.Lookup[func(string)](rt, "greet")
	if err != nil {
		t.Fatal(err)
	}

	greet("world")
	if got := out.String(); got != "hello world" {
		t.Errorf("output = %q, want %q", got, "hello world")
	}
}

// TestLookupReportsCallErrors routes a throw to the signature's error slot, and
// to the runtime's error sink when the signature has no slot to put it in.
func TestLookupReportsCallErrors(t *testing.T) {
	files := fstest.MapFS{"fail.php": {Data: []byte(`<?php
function boom() {
	throw new \Exception("detonated");
}
`)}}

	t.Run("into the error result", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)
		boom, err := runner.Lookup[func() (string, error)](rt, "boom")
		if err != nil {
			t.Fatal(err)
		}
		value, callErr := boom()
		if callErr == nil || !strings.Contains(callErr.Error(), "detonated") {
			t.Fatalf("error = %v, want one mentioning detonated", callErr)
		}
		if value != "" {
			t.Errorf("value = %q, want the zero value", value)
		}
	})

	t.Run("into the runtime sink", func(t *testing.T) {
		rt := lookupRuntime(t, nil, files)
		var recorded error
		rt.OnError(func(err error) { recorded = err })

		boom, err := runner.Lookup[func()](rt, "boom")
		if err != nil {
			t.Fatal(err)
		}
		boom()
		if recorded == nil || !strings.Contains(recorded.Error(), "detonated") {
			t.Fatalf("recorded = %v, want one mentioning detonated", recorded)
		}
	})
}

// TestLookupResolvesWithoutPrecompiling covers the runtime a host built and did
// nothing else with: the tree is walked on the first lookup that needs it.
func TestLookupResolvesWithoutPrecompiling(t *testing.T) {
	rt := lookupRuntime(t, nil, lookupTree)
	double, err := runner.Lookup[func(int) int](rt, "double")
	if err != nil {
		t.Fatal(err)
	}
	if got := double(21); got != 42 {
		t.Errorf("double(21) = %d, want 42", got)
	}
}

// TestLookupFindsADeclarationAlreadyRun covers the other entry: a host that ran
// its entrypoint has the declaration on the runtime already, and the lookup
// resolves against the function table without going near the tree.
func TestLookupFindsADeclarationAlreadyRun(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	program, err := rt.Load(`<?php function triple($n) { return $n * 3; }`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	triple, err := runner.Lookup[func(int) int](rt, "triple")
	if err != nil {
		t.Fatal(err)
	}
	if got := triple(14); got != 42 {
		t.Errorf("triple(14) = %d, want 42", got)
	}
}

// TestLookupConcurrent is the concurrency contract in executable form: one
// runtime per goroutine over one shared cache pair, which is what the server
// does per request. It is the test -race has to have.
func TestLookupConcurrent(t *testing.T) {
	const workers, calls = 16, 50

	includes := runner.NewIncludeCache()
	exprs := runner.NewExprCache()
	// Precompiled once for the process, the way a host warms its tree before it
	// serves anything.
	runner.Precompiler{Root: lookupTree, Includes: includes}.Run()

	var failures atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := range workers {
		go func() {
			defer wg.Done()

			rt := runner.New(io.Discard, runner.Options{RootFS: lookupTree})
			stdlib.Register(rt)
			rt.SetIncludeCache(includes)
			rt.SetExprCache(exprs)

			double, err := runner.Lookup[func(int) int](rt, "double")
			if err != nil {
				t.Errorf("worker %d: Lookup: %v", worker, err)
				failures.Add(1)
				return
			}
			handle, err := runner.Lookup[func(int) string](rt, `App\Users\handle`)
			if err != nil {
				t.Errorf("worker %d: Lookup: %v", worker, err)
				failures.Add(1)
				return
			}

			for call := range calls {
				if double(call) != call*2 {
					failures.Add(1)
				}
				if handle(call) == "" {
					failures.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if got := failures.Load(); got != 0 {
		t.Errorf("failed invocations = %d, want 0", got)
	}
}
