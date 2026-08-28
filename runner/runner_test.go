package runner_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

type snakeCaseMethodHost struct{}

func (*snakeCaseMethodHost) GetAll(context.Context) (any, error) {
	return "all", nil
}

func (*snakeCaseMethodHost) AffectedRows(context.Context) (any, error) {
	return "affected", nil
}

type constructorIDHost struct {
	id string
}

func (h *constructorIDHost) SetID(id string) { h.id = id }

func (h *constructorIDHost) GetID() string { return h.id }

type contextMethodHost struct{}

func (*contextMethodHost) Get(ctx context.Context, query string, args ...any) (any, error) {
	if ctx == nil {
		return nil, errors.New("missing context")
	}
	return query + ":" + fmt.Sprint(args[0]), nil
}

// run parses src, wires a tiny shim stdlib, executes, and returns the output.
func run(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	// minimal forwarded stdlib (the README's "bring your own stdlib" idea)
	rt.RegisterFunc("strlen", func(s string) int { return len(s) })
	rt.RegisterFunc("strtoupper", strings.ToUpper)
	rt.RegisterFunc("count", func(a any) int {
		if arr, ok := a.(interface{ Len() int }); ok {
			return arr.Len()
		}
		return 0
	})
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

func TestEchoAndConcat(t *testing.T) {
	got := run(t, `<?php echo "Hello, " . "world" . "!";`)
	if got != "Hello, world!" {
		t.Fatalf("got %q", got)
	}
}

func TestInlineHTML(t *testing.T) {
	got := run(t, `<h1><?php echo "hi"; ?></h1>`)
	if got != "<h1>hi</h1>" {
		t.Fatalf("got %q", got)
	}
}

func TestShebangIsIgnored(t *testing.T) {
	got := run(t, "#!/usr/bin/env phpscript\n<?php echo \"hi\";")
	if got != "hi" {
		t.Fatalf("got %q", got)
	}
}

func TestArithmeticAndVars(t *testing.T) {
	got := run(t, `<?php $a = 2; $b = 3; echo $a * $b + 1;`)
	if got != "7" {
		t.Fatalf("got %q", got)
	}
}

func TestArithmeticWithNumericStrings(t *testing.T) {
	prog, err := parser.Parse(`<?php echo $_GET["pageNumber"] + 0;`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	arr := model.NewArray()
	arr.Set("pageNumber", "1")
	rt.SetGlobal("_GET", arr)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); got != "1" {
		t.Fatalf("got %q", got)
	}
}

func TestForwardedFunction(t *testing.T) {
	got := run(t, `<?php echo strtoupper("abc") . strlen("hello");`)
	if got != "ABC5" {
		t.Fatalf("got %q", got)
	}
}

func TestConstructorResultReceivesVariableID(t *testing.T) {
	program, err := parser.Parse(`<?php $db = new Host; $alias = $db; echo $alias->get_id();`)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		new  func(io.Writer, runner.Options) *runner.Runtime
	}{
		{"interpreter", runner.New},
		{"flatstack", runner.NewFlatStack},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			instance := &constructorIDHost{}
			rt := test.new(&out, runner.Options{})
			rt.RegisterConstructor("Host", func() *constructorIDHost { return instance })
			if err := rt.Run(program); err != nil {
				t.Fatal(err)
			}
			if instance.id != "db" || out.String() != "db" {
				t.Fatalf("ID = %q, output = %q", instance.id, out.String())
			}
		})
	}
}

func TestNativeBoundMethodUsesUniformCallable(t *testing.T) {
	program, err := parser.Parse(`<?php $db = new Host; echo invoke_array($db->get, array("select", 7));`)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterConstructor("Host", func() *contextMethodHost { return &contextMethodHost{} })
	rt.RegisterFunc("invoke_array", func(fn func(...any) (any, error), args *model.Array) (any, error) {
		var values []any
		args.Range(func(_, value any) bool {
			values = append(values, value)
			return true
		})
		return fn(values...)
	})
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if out.String() != "select:7" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestGoMethodsMatchSnakeCasePHPNames(t *testing.T) {
	prog, err := parser.Parse(`<?php
$host = new Host;
echo $host->get_all();
echo ":" . $host->affected_rows();
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterConstructor("Host", func() *snakeCaseMethodHost { return &snakeCaseMethodHost{} })
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); got != "all:affected" {
		t.Fatalf("got %q", got)
	}
}

func TestAssignExportedGoField(t *testing.T) {
	type host struct {
		EnableTracing bool
	}
	instance := &host{}
	prog, err := parser.Parse(`<?php $host = new Host; $host->EnableTracing = true;`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rt := runner.New(io.Discard, runner.Options{})
	rt.RegisterConstructor("Host", func() *host { return instance })
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !instance.EnableTracing {
		t.Fatal("EnableTracing was not mutated")
	}
}

func TestForwardedFunctionContextIncludesScope(t *testing.T) {
	prog, err := parser.Parse(`<?php $local = "scope"; echo inspect_context("local");`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	type contextKey struct{}
	rt.SetContext(context.WithValue(t.Context(), contextKey{}, "lifecycle"))
	rt.RegisterFunc("inspect_context", func(ctx context.Context, name string) string {
		scope, ok := runner.ScopeFromContext(ctx)
		if !ok {
			return "missing scope"
		}
		value, _ := scope.Get(name)
		return ctx.Value(contextKey{}).(string) + ":" + value.(string)
	})
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); got != "lifecycle:scope" {
		t.Fatalf("got %q", got)
	}
}

func TestSharedExprCacheKeepsNestedExpressionsRuntimeLocal(t *testing.T) {
	cache := runner.NewExprCache()
	runCached := func(src string) string {
		t.Helper()
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		var out strings.Builder
		rt := runner.New(&out, runner.Options{})
		rt.SetExprCache(cache)
		rt.RegisterFunc("identity", func(v any) any { return v })
		if err := rt.Run(prog); err != nil {
			t.Fatalf("run: %v", err)
		}
		return out.String()
	}

	if got := runCached(`<?php $a = 1; echo identity($a++);`); got != "1" {
		t.Fatalf("first program got %q", got)
	}
	if got := runCached(`<?php $b = 10; echo identity($b++);`); got != "10" {
		t.Fatalf("second program got %q", got)
	}
}

func TestIfElse(t *testing.T) {
	got := run(t, `<?php $x = 5; if ($x > 3) { echo "big"; } else { echo "small"; }`)
	if got != "big" {
		t.Fatalf("got %q", got)
	}
}

func TestForLoop(t *testing.T) {
	got := run(t, `<?php for ($i = 0; $i < 3; $i = $i + 1) { echo $i; }`)
	if got != "012" {
		t.Fatalf("got %q", got)
	}
}

func TestPostIncrement(t *testing.T) {
	got := run(t, `<?php $i = 0; echo $i++; echo $i; for (; $i < 4; $i++) { echo $i; }`)
	if got != "01123" {
		t.Fatalf("got %q", got)
	}
}

func TestForeachArray(t *testing.T) {
	got := run(t, `<?php $xs = array(10, 20, 30); foreach ($xs as $v) { echo $v . ","; }`)
	if got != "10,20,30," {
		t.Fatalf("got %q", got)
	}
}

func TestForeachKeyValue(t *testing.T) {
	got := run(t, `<?php $m = array("a" => 1, "b" => 2); foreach ($m as $k => $v) { echo $k . "=" . $v . ";"; }`)
	if got != "a=1;b=2;" {
		t.Fatalf("got %q", got)
	}
}

func TestForeachIndexedTargets(t *testing.T) {
	got := run(t, `<?php $m = array("a" => 1, "b" => 2); $out = array(); foreach ($m as $out['key'] => $out['value']) { echo $out['key'] . "=" . $out['value'] . ";"; }`)
	if got != "a=1;b=2;" {
		t.Fatalf("got %q", got)
	}
}

func TestForeachWithoutParens(t *testing.T) {
	got := run(t, `<?php $m = array("a" => 1, "b" => 2); foreach $m as $k => $v { echo $k . "=" . $v . ";"; }`)
	if got != "a=1;b=2;" {
		t.Fatalf("got %q", got)
	}
}

func TestArrayIndexAndAppend(t *testing.T) {
	got := run(t, `<?php $a = array(); $a[] = "x"; $a[] = "y"; echo $a[0] . $a[1] . count($a);`)
	if got != "xy2" {
		t.Fatalf("got %q", got)
	}
}

func TestUserFunction(t *testing.T) {
	got := run(t, `<?php function add($a, $b) { return $a + $b; } echo add(4, 5);`)
	if got != "9" {
		t.Fatalf("got %q", got)
	}
}

func TestFunctionDefault(t *testing.T) {
	got := run(t, `<?php function greet($name = "world") { return "hi " . $name; } echo greet();`)
	if got != "hi world" {
		t.Fatalf("got %q", got)
	}
}

func TestClassAndMethod(t *testing.T) {
	src := `<?php
class Counter {
	var $n = 0;
	function inc() { $this->n = $this->n + 1; return $this->n; }
}
$c = new Counter;
echo $c->inc() . $c->inc();`
	got := run(t, src)
	if got != "12" {
		t.Fatalf("got %q", got)
	}
}

func TestExternalMethodSyntax(t *testing.T) {
	src := `<?php
class Box { var $v = 0; }
function Box::set($x) { $this->v = $x; }
function Box::get() { return $this->v; }
$b = new Box;
$b->set(42);
echo $b->get();`
	got := run(t, src)
	if got != "42" {
		t.Fatalf("got %q", got)
	}
}

func TestConstructor(t *testing.T) {
	src := `<?php
class Greeter {
	var $who;
	function Greeter($name) { $this->who = $name; }
	function hello() { return "hello " . $this->who; }
}
$g = new Greeter("amp");
echo $g->hello();`
	got := run(t, src)
	if got != "hello amp" {
		t.Fatalf("got %q", got)
	}
}

// TestBuiltinOverride proves expr-lang builtins are disabled so a registered
// PHP function may reuse a name that expr ships as a builtin (e.g. `count`).
func TestBuiltinOverride(t *testing.T) {
	prog, err := parser.Parse(`<?php echo count(array(1, 2, 3, 4));`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	// Our own count: PHP arrays count their entries.
	rt.RegisterFunc("count", func(a any) int {
		if arr, ok := a.(interface{ Len() int }); ok {
			return arr.Len()
		}
		return 0
	})
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "4" {
		t.Fatalf("got %q, want 4", out.String())
	}
}

func TestTernary(t *testing.T) {
	got := run(t, `<?php $x = 0; echo $x ? "yes" : "no";`)
	if got != "no" {
		t.Fatalf("got %q", got)
	}
}

// TestTokenGetAllFromVM proves the parser's PHP-compatible tokenizer is usable
// from transpiled PHP code: foreach over the tokens, is_array() to distinguish
// the array form, $v[0]/$v[1] access, and token_name(): exactly the shape
// minitpl's _split_exp depends on.
func TestTokenGetAllFromVM(t *testing.T) {
	prog, err := parser.Parse(`<?php
$out = "";
foreach (token_get_all($code) as $v) {
	if (is_array($v)) {
		$out = $out . token_name($v[0]) . ":" . $v[1] . "\n";
	}
}
echo $out;`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterFunc("token_get_all", parser.TokenGetAll)
	rt.RegisterFunc("token_name", parser.TokenName)
	// Shape-agnostic, like the real stdlib is_array: TokenGetAll returns a
	// []any of []any, not an *model.Array.
	rt.RegisterFunc("is_array", model.IsCollection)
	// minitpl-style wrapped expression with the "." -> "__1" marker.
	rt.SetGlobal("code", `<?php if ($this->_vars) { ?>`)

	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()
	for _, want := range []string{"T_VARIABLE:$this", "T_OBJECT_OPERATOR:->", "T_IF:if"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
}

func TestNamespaceDeclarationPlacement(t *testing.T) {
	for _, src := range []string{
		`<?php echo "side effect"; namespace App; class User {}`,
		`<?php function outer() { namespace App; }`,
	} {
		if _, err := parser.Parse(src); err == nil {
			t.Fatalf("expected invalid namespace placement for %q", src)
		}
	}
}

func TestClassExistsPropagatesAutoloaderError(t *testing.T) {
	rt := runner.New(nil, runner.Options{})
	want := errors.New("autoload failed")
	rt.RegisterAutoloader(func(string) error { return want }, false)

	exists, err := rt.ClassExists("Missing", true)
	if exists {
		t.Fatal("missing class unexpectedly exists")
	}
	if !errors.Is(err, want) {
		t.Fatalf("got error %v, want %v", err, want)
	}
}

// autoloadFS is a source tree carrying the folder convention: an autoload/
// directory with one namespaced class and one at its root, a second folder to
// point the option at, and a file outside both that no class name may reach.
func autoloadFS() fstest.MapFS {
	file := func(src string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(src)}
	}
	return fstest.MapFS{
		"autoload/Acme/Thing.php": file("<?php\nnamespace Acme;\nclass Thing { public function name() { return \"folder\"; } }\n"),
		"autoload/Bare.php":       file("<?php\nclass Bare {}\n"),
		"lib/Acme/Thing.php":      file("<?php\nnamespace Acme;\nclass Thing { public function name() { return \"lib\"; } }\n"),
		"secret.php":              file("<?php\nclass Secret {}\n"),
	}
}

func TestFolderAutoloadResolvesNamespacedClass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		autoload string
		class    string
	}{
		{name: "default folder", class: "Acme\\Thing"},
		{name: "folder root", class: "Bare"},
		{name: "named folder", autoload: "lib", class: "Acme\\Thing"},
		{name: "leading separator", class: "\\Acme\\Thing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := runner.New(io.Discard, runner.Options{RootFS: autoloadFS(), Autoload: tc.autoload})
			exists, err := rt.ClassExists(tc.class, true)
			if err != nil {
				t.Fatalf("class_exists: %v", err)
			}
			if !exists {
				t.Fatalf("class %q did not autoload from the folder", tc.class)
			}
		})
	}
}

// TestFolderAutoloadRefusesTraversal pins that a class name is not a path.
// class_exists takes an arbitrary string, so a name whose segments are not PHP
// labels must produce a miss rather than a file read outside the folder.
func TestFolderAutoloadRefusesTraversal(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{RootFS: autoloadFS()})
	for _, class := range []string{
		"../secret",
		"..\\secret",
		"Acme\\..\\..\\secret",
		"Acme/Thing",
		"",
	} {
		exists, err := rt.ClassExists(class, true)
		if err != nil {
			t.Fatalf("class_exists %q: %v", class, err)
		}
		if exists {
			t.Fatalf("class %q resolved, want a miss", class)
		}
	}
}

// TestFolderAutoloadInertWithoutDirectory pins the off switch: the convention is
// disabled by not having the directory, not by a configuration key.
func TestFolderAutoloadInertWithoutDirectory(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts runner.Options
	}{
		{name: "no autoload directory", opts: runner.Options{RootFS: fstest.MapFS{}}},
		{name: "no source fs", opts: runner.Options{}},
		{name: "directory named but absent", opts: runner.Options{RootFS: autoloadFS(), Autoload: "nowhere"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := runner.New(io.Discard, tc.opts)
			exists, err := rt.ClassExists("Acme\\Thing", true)
			if err != nil {
				t.Fatalf("class_exists: %v", err)
			}
			if exists {
				t.Fatal("class resolved with no autoload directory")
			}
		})
	}
}

// TestFolderAutoloadRunsAfterRegisteredQueue pins the ordering the folder
// convention is built on: it is a fallback, so a callback the script registered
// is consulted first even for a class the folder could have answered.
func TestFolderAutoloadRunsAfterRegisteredQueue(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{RootFS: autoloadFS()})
	var seen []string
	rt.RegisterAutoloader(func(class string) error {
		seen = append(seen, class)
		return nil
	}, false)

	exists, err := rt.ClassExists("Acme\\Thing", true)
	if err != nil {
		t.Fatalf("class_exists: %v", err)
	}
	if !exists {
		t.Fatal("class did not autoload from the folder")
	}
	if len(seen) != 1 || seen[0] != "Acme\\Thing" {
		t.Fatalf("registered autoloader saw %v, want one call for Acme\\Thing", seen)
	}
}

// TestFolderAutoloadIsPerSession pins that what the folder loaded belongs to one
// request: the next session starts without it and autoloads it again.
func TestFolderAutoloadIsPerSession(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{RootFS: autoloadFS()})
	if exists, err := rt.ClassExists("Acme\\Thing", true); err != nil || !exists {
		t.Fatalf("first session: exists=%v err=%v", exists, err)
	}

	rt.ResetSession(io.Discard, nil)
	if exists, err := rt.ClassExists("Acme\\Thing", false); err != nil || exists {
		t.Fatalf("after reset without autoload: exists=%v err=%v", exists, err)
	}
	if exists, err := rt.ClassExists("Acme\\Thing", true); err != nil || !exists {
		t.Fatalf("second session: exists=%v err=%v", exists, err)
	}
}

// TestRegisterFuncBetweenRuns proves the reusable evaluation environment picks
// up function-table changes: a Runtime that has already evaluated expressions
// must see a function registered (or replaced) afterwards.
func TestRegisterFuncBetweenRuns(t *testing.T) {
	first, err := parser.Parse(`<?php echo greet("a");`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	second, err := parser.Parse(`<?php echo greet("b") . shout("c");`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterFunc("greet", func(s string) string { return "hello " + s })
	if err := rt.Run(first); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "hello a" {
		t.Fatalf("got %q, want %q", out.String(), "hello a")
	}

	out.Reset()
	// Replace one function and add another after the environment was built.
	rt.RegisterFunc("greet", func(s string) string { return "hi " + s })
	rt.RegisterFunc("shout", strings.ToUpper)
	if err := rt.Run(second); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "hi bC" {
		t.Fatalf("got %q, want %q", out.String(), "hi bC")
	}
}

// TestNestedEvalScopes proves that reused environments never leak a scope: a
// user function called from inside an expression must see its own frame, and
// the caller's variables must survive the nested evaluation.
func TestNestedEvalScopes(t *testing.T) {
	got := run(t, `<?php
function inner($x) {
	$local = $x * 2;
	return $local;
}
function outer($x) {
	$local = "outer";
	$sum = inner($x) + inner($x + 1);
	return $local . ":" . $sum;
}
$local = "top";
echo outer(2) . "|" . $local;`)
	if got != "outer:10|top" {
		t.Fatalf("got %q, want %q", got, "outer:10|top")
	}
}

// TestLazyEnvInstallsFunctionsOnDemand covers the environments' on-demand
// population. A registered function is wrapped into an environment the first
// time an expression evaluated with that environment calls it, so the places a
// call can hide from a naive "top-level expressions only" scan are what matters:
// a closure body, a user function body, a method body, a nested/recursive call
// and a by-reference call.
func TestLazyEnvInstallsFunctionsOnDemand(t *testing.T) {
	src := `<?php
function shout($s) {
	return strtoupper($s) . strlen($s);
}
class Box {
	public $items = [];
	public function add($v) {
		$this->items[] = shout($v);
		return $this;
	}
	public function render() {
		return implode(",", $this->items);
	}
}
function depth($n) {
	if ($n <= 0) {
		return strtoupper("done");
	}
	return depth($n - 1) . strlen("x");
}
$b = new Box();
$b->add("a");
$b->add("bb");
$sizes = apply(["cc", "d"], function ($x) { return shout($x) . strlen($x); });
echo $b->render() . "|" . depth(3) . "|" . implode("-", $sizes);`

	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterFunc("strlen", func(s string) int { return len(s) })
	rt.RegisterFunc("strtoupper", strings.ToUpper)
	rt.RegisterFunc("implode", func(sep string, items any) string {
		var parts []string
		model.RangeValues(items, func(_, v any) bool {
			parts = append(parts, fmt.Sprint(v))
			return true
		})
		return strings.Join(parts, sep)
	})
	// apply drives a PHP closure from Go, which re-enters Eval with a fresh
	// environment while the caller's is still live.
	rt.RegisterFunc("apply", func(items any, fn func(...any) (any, error)) ([]any, error) {
		var out []any
		var err error
		model.RangeValues(items, func(_, v any) bool {
			var mapped any
			if mapped, err = fn(v); err != nil {
				return false
			}
			out = append(out, mapped)
			return true
		})
		return out, err
	})
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "A1,BB2|DONE111|CC22-D11"; out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

// TestLazyEnvSeesFunctionsRegisteredAfterPooling asserts that an environment
// which has already been used (and returned to the free list) still resolves a
// function registered afterwards, and picks up a replacement implementation of a
// name it had already installed.
func TestLazyEnvSeesFunctionsRegisteredAfterPooling(t *testing.T) {
	warm, err := parser.Parse(`<?php echo greet("a");`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	later, err := parser.Parse(`<?php echo greet("b") . shout("c") . nested();`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterFunc("greet", func(s string) string { return "hello " + s })
	if err := rt.Run(warm); err != nil {
		t.Fatalf("run: %v", err)
	}
	out.Reset()

	// Registered after every environment this runtime holds has been built,
	// used and pooled.
	rt.RegisterFunc("greet", func(s string) string { return "hi " + s })
	rt.RegisterFunc("shout", strings.ToUpper)
	rt.RegisterFunc("nested", func() string { return "!" })
	if err := rt.Run(later); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "hi bC!"; out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

// TestLazyEnvRecursiveEvalNestsDeeperThanTheFreeList drives evaluation deeper
// than the pooled environment free list, so environments are built, nested,
// discarded and reused while a call to a lazily installed function is live at
// every level.
func TestLazyEnvRecursiveEvalNestsDeeperThanTheFreeList(t *testing.T) {
	got := run(t, `<?php
function down($n) {
	if ($n <= 0) {
		return strtoupper("x");
	}
	return down($n - 1) . strlen("ab");
}
echo down(40);`)
	if want := "X" + strings.Repeat("2", 40); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestLazyEnvUndefinedFunctionStillErrors asserts the diagnostic for a call to
// a function that is not registered: it is a runtime error, not a silent nil.
func TestLazyEnvUndefinedFunctionStillErrors(t *testing.T) {
	prog, err := parser.Parse(`<?php echo no_such_function(1);`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rt := runner.New(io.Discard, runner.Options{})
	if err := rt.Run(prog); err == nil {
		t.Fatal("calling an unregistered function: want error, got nil")
	}
}

// TestInterfaceContractRaisesOnBothBackends pins the rule that made the
// previous attempt at class metadata a mistake: a check wired into one backend
// and not the other means the two disagree about what a program does. A
// violated contract must be the same RuntimeException whichever engine runs it,
// raised before any output is produced.
func TestInterfaceContractRaisesOnBothBackends(t *testing.T) {
	prog, err := parser.Parse(`<?php
interface Reader {
	function get($key);
	function has($key);
}

class Store implements Reader {
	function get($key) {
		return $key;
	}
}

echo "unreachable";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	const want = "class Store does not declare method has() required by interface Reader"
	for _, test := range []struct {
		name string
		new  func(io.Writer, runner.Options) *runner.Runtime
	}{
		{"interpreter", runner.New},
		{"flatstack", runner.NewFlatStack},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			err := test.new(&out, runner.Options{}).Run(prog)
			if err == nil {
				t.Fatalf("a violated contract ran, with output %q", out.String())
			}
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
			var thrown *runner.RuntimeException
			if !errors.As(err, &thrown) {
				t.Errorf("error is %T, want a *runner.RuntimeException a catch clause takes", err)
			}
			if out.String() != "" {
				t.Errorf("output = %q, want none: the check runs before the program does", out.String())
			}
		})
	}
}

// A class satisfying its contract runs on either backend, and gains no member
// from the interface: the interface declares no body, so the class answers only
// with what it wrote. `instanceof` does consult the list of names the class
// declared, which is a name comparison rather than an inherited member.
func TestInterfaceContractSatisfied(t *testing.T) {
	prog, err := parser.Parse(`<?php
interface Reader {
	function get($key);
}

interface Listing extends Reader {
	function keys();
}

class Store implements Listing {
	function get($key) {
		return "got:" . $key;
	}

	function keys() {
		return "keys";
	}
}

$store = new Store;
echo $store->get("a"), " ", $store->keys(), " ", $store instanceof Reader ? "yes" : "no";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, test := range []struct {
		name string
		new  func(io.Writer, runner.Options) *runner.Runtime
	}{
		{"interpreter", runner.New},
		{"flatstack", runner.NewFlatStack},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			if err := test.new(&out, runner.Options{}).Run(prog); err != nil {
				t.Fatalf("run: %v", err)
			}
			if want := "got:a keys yes"; out.String() != want {
				t.Fatalf("got %q, want %q", out.String(), want)
			}
		})
	}
}
