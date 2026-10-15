package flatstack_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/expr"
	"github.com/titpetric/phpscript/stdlib"
)

// parse compiles one source string, failing the test when the source itself
// is wrong.
func parse(t *testing.T, source string) *model.Program {
	t.Helper()

	program, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	return program
}

// TestExitError covers the alias the exit error travels under. The embedding
// API is runner's, aliased and never wrapped, so a value a host builds under
// the flatstack name is accepted everywhere runner's own type is, and the
// wording and status are runner's.
func TestExitError(t *testing.T) {
	t.Parallel()
	err := &flatstack.ExitError{Code: 3}

	if got, want := err.Error(), "exit(3)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := err.ScriptExit(); got != 3 {
		t.Errorf("ScriptExit() = %d, want 3", got)
	}

	// The alias is the same type, so one value satisfies both spellings and a
	// host swapping runner for flatstack changes its import alone.
	var same *runner.ExitError = err
	if same.Code != 3 {
		t.Errorf("the runner spelling carries code %d, want 3", same.Code)
	}
}

// TestIsExit covers how a host separates a script that ended from one that
// failed. A program calling exit() returns the error through Run, and the
// output written before the call stays written.
func TestIsExit(t *testing.T) {
	t.Parallel()
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	stdlib.Register(runtime)

	err := runtime.Run(parse(t, `<?php echo "before"; exit(4);`))
	exit, ok := flatstack.IsExit(err)
	if !ok {
		t.Fatalf("IsExit() = false for %v, want the exit", err)
	}
	if exit.Code != 4 {
		t.Errorf("the exit carries code %d, want 4", exit.Code)
	}
	if got := output.String(); got != "before" {
		t.Errorf("output = %q, want what was echoed before the exit", got)
	}

	// Through a wrapper: a host several frames up reads it this way.
	exit, ok = flatstack.IsExit(fmt.Errorf("serving: %w", &flatstack.ExitError{Code: 7}))
	if !ok || exit.Code != 7 {
		t.Errorf("IsExit() over a wrapped exit = %+v, %t, want code 7 and true", exit, ok)
	}
	if _, ok := flatstack.IsExit(errors.New("ordinary failure")); ok {
		t.Error("IsExit() = true for an error that is not an exit")
	}
}

// TestExprCache covers the two tables the cache holds and the sharing it
// exists for. Flat bytecode is keyed by parsed program identity, so
// EnsureFlat compiles once and a second call reads it back; compiled
// expressions are keyed by AST node identity. Len counts the expression half,
// so it stays zero while only bytecode has been stored.
func TestExprCache(t *testing.T) {
	t.Parallel()
	cache := flatstack.NewExprCache()
	program := parse(t, `<?php $n = 2; echo $n * 3;`)

	if got := cache.Len(); got != 0 {
		t.Fatalf("Len() = %d on a fresh cache, want 0", got)
	}
	for _, call := range []string{"first", "second"} {
		if err := cache.EnsureFlat(program); err != nil {
			t.Fatalf("EnsureFlat, %s call: %v", call, err)
		}
	}

	// Two runtimes handed one cache run the bytecode it already holds, and
	// both write the same output.
	for _, run := range []string{"first", "second"} {
		var output strings.Builder
		runtime := flatstack.New(&output, flatstack.Options{})
		runtime.SetExprCache(cache)
		if err := runtime.Run(program); err != nil {
			t.Fatalf("%s run: %v", run, err)
		}
		if got := output.String(); got != "6" {
			t.Errorf("%s run wrote %q, want %q", run, got, "6")
		}
	}

	// The expression half is keyed by the node itself, so one node stored is
	// one entry and reading it back returns the same compiled value.
	node := &model.Lit{Value: int64(7)}
	compiled, err := expr.CompileExpr(node, &expr.Helpers{})
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	cache.SetExpr(node, compiled)
	got, ok := cache.GetExpr(node)
	if !ok || got != compiled {
		t.Errorf("GetExpr() = %p, %t, want the stored %p and true", got, ok, compiled)
	}
	if n := cache.Len(); n != 1 {
		t.Errorf("Len() = %d, want 1", n)
	}

	// Clear drops both tables, so a host holding the cache for the life of
	// the process can release what it retained.
	cache.Clear()
	if _, ok := cache.GetExpr(node); ok || cache.Len() != 0 {
		t.Error("the cache still answers after Clear()")
	}
}

// TestIncludeCache covers the parsed-program cache includes resolve through.
// A program is stored under its cleaned path and read back as the same value,
// because the runner treats a parsed program as immutable and shares it
// between the callers that only read the AST.
func TestIncludeCache(t *testing.T) {
	t.Parallel()
	cache := flatstack.NewIncludeCache()
	program := parse(t, `<?php echo "included";`)

	if _, ok := cache.Get("lib/greet.php"); ok {
		t.Fatal("Get() reported a hit on a fresh cache")
	}

	cache.Set("lib/greet.php", program)
	got, ok := cache.Get("lib/greet.php")
	if !ok {
		t.Fatal("Get() reported a miss for a path that was just stored")
	}
	if got != program {
		t.Errorf("Get() = %p, want the stored %p", got, program)
	}
	if n := cache.Len(); n != 1 {
		t.Errorf("Len() = %d, want 1", n)
	}

	// Account records what precompiling read off disk, so a host can report
	// how much source it holds resident.
	cache.Account(1024)
	if got := cache.Precompiled(); got != 1024 {
		t.Errorf("Precompiled() = %d, want 1024", got)
	}

	cache.Clear()
	if _, ok := cache.Get("lib/greet.php"); ok || cache.Len() != 0 {
		t.Error("the cache still answers after Clear()")
	}
}

// TestIncludeFunc covers the resolver a host wires in. The runner holds no
// filesystem and no parser, so an include is resolved by the function the
// host set, and whatever it returns is executed in place of reading a file.
func TestIncludeFunc(t *testing.T) {
	t.Parallel()
	asked := ""
	var resolve flatstack.IncludeFunc = func(path string) (*model.Program, error) {
		asked = path
		return parser.Parse(`<?php echo "[library]";`)
	}

	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetIncludeResolver(resolve)

	if err := runtime.Run(parse(t, `<?php echo "a"; include "lib/greet.php"; echo "b";`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if asked != "lib/greet.php" {
		t.Errorf("the resolver was called with %q, want the path the script named", asked)
	}
	if got := output.String(); got != "a[library]b" {
		t.Errorf("output = %q, want the include between the two echoes", got)
	}
}

// TestScope covers the variable table one execution frame holds. PHP has no
// block scoping, so one flat table per frame is the whole model: a name set
// is readable, an unset name is gone, and DefinedVars reports the table as
// get_defined_vars() does.
func TestScope(t *testing.T) {
	t.Parallel()
	scope := flatstack.NewScope()

	if _, ok := scope.Get("total"); ok {
		t.Error("Get() reported a hit on a fresh scope")
	}

	scope.Set("total", int64(42))
	got, ok := scope.Get("total")
	if !ok || got != int64(42) {
		t.Errorf("Get(total) = %v, %t, want int64(42), true", got, ok)
	}

	scope.Set("name", "ada")
	if vars := scope.DefinedVars(); vars["total"] != int64(42) || vars["name"] != "ada" {
		t.Errorf("DefinedVars() = %v, want both names", vars)
	}

	scope.Unset("total")
	if _, ok := scope.Get("total"); ok {
		t.Error("Get() still answers an unset name")
	}
}

// TestContext covers the request context a host fills itself. NewContext
// allocates the maps a host writes into directly, so a host that sets a query
// field reads it back through the accessor a superglobal is rendered from,
// with no request behind it.
func TestContext(t *testing.T) {
	t.Parallel()
	ctx := flatstack.NewContext()

	ctx.Get["q"] = "phpscript"
	ctx.Cookie["sid"] = "abc"
	ctx.Headers["X-Request-Id"] = "r1"

	if got := ctx.GetMap()["q"]; got != "phpscript" {
		t.Errorf("GetMap()[q] = %q, want %q", got, "phpscript")
	}
	if got := ctx.CookieMap()["sid"]; got != "abc" {
		t.Errorf("CookieMap()[sid] = %q, want %q", got, "abc")
	}
	if got := ctx.HeaderMap()["X-Request-Id"]; got != "r1" {
		t.Errorf("HeaderMap()[X-Request-Id] = %q, want %q", got, "r1")
	}

	// The alias is runner's type, so the same value is what either package
	// returns.
	var same runner.Context = ctx
	if same.GetMap()["q"] != "phpscript" {
		t.Error("the runner spelling reads a different value")
	}
}

// TestFromRequest covers the context a served request decodes to, as a script
// reads it: the query by name, the cookies by name, and the CGI names derived
// off the request without a map being built for them.
func TestFromRequest(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("GET", "http://example.com/page?q=phpscript&n=2", nil)
	request.AddCookie(&http.Cookie{Name: "sid", Value: "abc"})
	request.Header.Set("Accept-Encoding", "gzip")

	ctx := flatstack.FromRequest(request)

	if got := ctx.GetMap()["q"]; got != "phpscript" {
		t.Errorf("GetMap()[q] = %q, want %q", got, "phpscript")
	}
	if got := ctx.GetMap()["n"]; got != "2" {
		t.Errorf("GetMap()[n] = %q, want %q", got, "2")
	}
	if got := ctx.CookieMap()["sid"]; got != "abc" {
		t.Errorf("CookieMap()[sid] = %q, want %q", got, "abc")
	}
	if got := ctx.ServerMap().Read("REQUEST_METHOD"); got != "GET" {
		t.Errorf("$_SERVER[REQUEST_METHOD] = %v, want GET", got)
	}
	if got := ctx.ServerMap().Read("QUERY_STRING"); got != "q=phpscript&n=2" {
		t.Errorf("$_SERVER[QUERY_STRING] = %v, want the raw query", got)
	}
}

// TestScopeFromContext covers what a registered Go callable reads off the
// context it is handed: the calling PHP frame. The runtime attaches
// it per call, so a binding reaches the caller's variables without the script
// passing them.
func TestScopeFromContext(t *testing.T) {
	t.Parallel()
	var (
		found bool
		total any
	)

	runtime := flatstack.New(io.Discard, flatstack.Options{})
	runtime.RegisterFunc("read_total", func(ctx context.Context) (string, error) {
		scope, ok := flatstack.ScopeFromContext(ctx)
		found = ok
		if ok {
			total, _ = scope.Get("total")
		}
		return "", nil
	})

	if err := runtime.Run(parse(t, `<?php $total = 42; read_total();`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !found {
		t.Fatal("ScopeFromContext() = false inside a registered callable, want the frame")
	}
	if total != int64(42) {
		t.Errorf("the frame reads $total as %#v, want int64(42)", total)
	}

	// A context nobody attached a frame to answers false, so a binding called
	// from Go and not from a script can tell.
	if _, ok := flatstack.ScopeFromContext(context.Background()); ok {
		t.Error("ScopeFromContext() = true for a plain context")
	}
}
