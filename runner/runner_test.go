package runner_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

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
// the array form, $v[0]/$v[1] access, and token_name() — exactly the shape
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
	rt.RegisterFunc("is_array", func(a any) bool {
		_, ok := a.(*model.Array)
		return ok
	})
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
