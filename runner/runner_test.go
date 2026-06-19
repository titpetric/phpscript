package runner_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// run parses src, wires a tiny shim stdlib, executes, and returns the output.
func run(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out)
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

func TestArithmeticAndVars(t *testing.T) {
	got := run(t, `<?php $a = 2; $b = 3; echo $a * $b + 1;`)
	if got != "7" {
		t.Fatalf("got %q", got)
	}
}

func TestForwardedFunction(t *testing.T) {
	got := run(t, `<?php echo strtoupper("abc") . strlen("hello");`)
	if got != "ABC5" {
		t.Fatalf("got %q", got)
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
	rt := runner.New(&out)
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
	rt := runner.New(&out)
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
