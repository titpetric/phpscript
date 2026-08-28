package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/lint"
)

func TestFlatstackLinterCompatibility(t *testing.T) {
	compatibleSrc := `<?php
	$a = 1 + 2;
	echo $a;
	?>`

	diag, err := lint.FlatstackFile("compatible.php", compatibleSrc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diag.Message != "[flatstack compatible] 100% compatible with flatstack bytecode engine" {
		t.Fatalf("expected compatible message, got %s", diag.Message)
	}

	// compact() needs to read the caller scope by name, which the bytecode
	// engine has no representation for.
	unsupportedSrc := `<?php
	$a = 1;
	$out = compact("a");
	?>`

	diag, err = lint.FlatstackFile("unsupported.php", unsupportedSrc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diag.Message == "[flatstack compatible] 100% compatible with flatstack bytecode engine" {
		t.Fatalf("expected unsupported diagnostic for compact(), got %s", diag.Message)
	}
}

func TestPathsReportsParseErrorsAndContinues(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"invalid.php": `<?php class Foo {`,
		"lint.php":    `<?php if ($value = true) {}`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	diags, err := lint.Paths([]string{dir})
	if err != nil {
		t.Fatalf("Paths returned a parser error instead of continuing: %v", err)
	}
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want parser and lint diagnostics: %+v", len(diags), diags)
	}
	if got := diags[0].Message; len(got) < len("parse error:") || got[:len("parse error:")] != "parse error:" {
		t.Fatalf("first diagnostic = %q, want parse error", got)
	}
	if got := diags[1].Message; got != "assignment in conditional statement" {
		t.Fatalf("second diagnostic = %q, want assignment diagnostic", got)
	}
}

func TestPathsLintsPHPSectionOfPHPTFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assignment.phpt")
	src := `name: assignment condition
description: Lint the PHP section only.
---
<?php
if ($value = true) {}
---
done
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	diags, err := lint.Paths([]string{path})
	if err != nil {
		t.Fatalf("Paths returned an error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want assignment diagnostic: %+v", len(diags), diags)
	}
	if got := diags[0].Message; got != "assignment in conditional statement" {
		t.Fatalf("diagnostic = %q, want assignment diagnostic", got)
	}
	if got := diags[0].Line; got != 5 {
		t.Fatalf("diagnostic line = %d, want physical fixture line 5", got)
	}
}

func TestFileReportsChainedAssignment(t *testing.T) {
	src := `<?php
$dba = $dbb = new Database();
$one = new Database();
$a = $b = $c = compute();
$paren = ($x = $handle);
$obj->left = $obj->right = $shared;
function scoped() { $p = $q = new Database(); }
class Holder { function method() { $r = $s = load(); } }
foreach (array(1) as $v) { $t = $u = $v; }
for ($i = 0; $i < 1; $i++) { $w = $y = $z; }
if (true) { $m = $n = $o; }
function compute() { return 1; }
function load() { return 1; }
`
	diags, err := lint.File("chain.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}

	// Every chained statement whose value is a handle, or a name whose type the
	// source does not settle, is reported once, whatever scope it sits in, and
	// a chain of three names is still one finding rather than one per link.
	wantLines := []int{2, 4, 5, 6, 7, 8, 9, 10, 11}
	if len(diags) != len(wantLines) {
		t.Fatalf("got %d diagnostics, want %d: %+v", len(diags), len(wantLines), diags)
	}
	for i, want := range wantLines {
		if diags[i].Line != want {
			t.Errorf("diagnostic %d line = %d, want %d (%q)", i, diags[i].Line, want, diags[i].Message)
		}
		if got := diags[i].Message; got != "chained assignment binds one value to several names" {
			t.Errorf("diagnostic %d message = %q", i, got)
		}
	}
}

// A chain that ends in a literal is left alone. A scalar is immutable, so
// neither name can reach the other through it, and an array literal is split
// into one allocation per name by the parser, so there is nothing left to
// share. `$r['y'] = $r['m'] = '00'` is the shape this comes from.
func TestFileAcceptsChainedLiterals(t *testing.T) {
	src := `<?php
$r = array();
$r['y'] = $r['m'] = $r['d'] = '00';
$inlines = $blocks = array();
$rows = $cols = [1, 2, 3];
function scopedAlloc() { $g = $h = array("k" => "v"); }
$a = $b = $c = 1;
$paren = ($x = 5);
$obj->left = $obj->right = "v";
$i = $j = "count: $n";
$k = $l = null;
$m = $n = true;
$o = $p = -1.5;
function scoped() { $q = $s = 0; }
`
	diags, err := lint.File("scalar.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("got %d diagnostics for chained literals: %+v", len(diags), diags)
	}
}

func TestFileAcceptsSingleAssignment(t *testing.T) {
	src := `<?php
$value = array();
$sum = 1 + 2;
$result = compute($value);
$value[] = $sum;
$value["k"] = $sum;
function compute($v) { return $v; }
`
	diags, err := lint.File("single.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("got %d diagnostics for unchained assignments: %+v", len(diags), diags)
	}
}

// A violated interface contract is a lint finding on the line the class was
// declared on, so a file is told what it is missing before it is run.
func TestFileReportsInterfaceContract(t *testing.T) {
	src := "<?php\ninterface Reader {\n\tfunction get($key);\n\tfunction has($key);\n}\n\nclass Store implements Reader {\n\tfunction get($key) {\n\t\treturn $key;\n\t}\n}\n"
	diags, err := lint.File("store.php", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one", diags)
	}
	want := "store.php:7: class Store does not declare method has() required by interface Reader"
	if got := diags[0].String(); got != want {
		t.Errorf("diagnostic = %q, want %q", got, want)
	}
}

// A class satisfying its contract, and one naming an interface no declaration
// defines, both lint clean: the second is a built-in name such as Countable,
// which phpscript does not declare.
func TestFileAcceptsSatisfiedAndUndeclaredInterfaces(t *testing.T) {
	src := "<?php\ninterface Reader {\n\tfunction get($key);\n}\n\nclass Store implements Reader, Countable {\n\tfunction get($key) {\n\t\treturn $key;\n\t}\n}\n"
	diags, err := lint.File("store.php", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %v, want none", diags)
	}
}

// global and extends are documented won't-implements (docs/design.md) that
// parse and do nothing, so the linter is where a port hears about them before
// the behaviour goes wrong at a distance.
func TestFileReportsGlobalAndExtends(t *testing.T) {
	src := `<?php
class Animal {
	function speak() { return "..."; }
}
class Dog extends Animal {
	function fetch() {
		global $ball;
		return $ball;
	}
}
function f() {
	GLOBAL $x;
}
`
	diags, err := lint.File("port.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}

	want := []string{
		"port.php:5: extends is a no-op: Dog inherits nothing from Animal; declare the members it uses",
		"port.php:7: global is a no-op: the variable stays unset; pass the collaborator as a parameter",
		"port.php:12: global is a no-op: the variable stays unset; pass the collaborator as a parameter",
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

// Interface extends widens the declaration contract, which instanceof does
// follow, so it is not the no-op the class form is and lints clean.
func TestFileAcceptsInterfaceExtends(t *testing.T) {
	src := "<?php\ninterface Reader {\n\tfunction get($key);\n}\ninterface Store extends Reader {\n\tfunction put($key, $value);\n}\n"
	diags, err := lint.File("iface.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %v, want none", diags)
	}
}

// A JSON_* constant is reported as a warning. The name is not defined, so it
// arrives as null and json_encode ignores it: the call runs and encodes, and
// what the author loses is the formatting they asked for.
func TestLintJSONFlags(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		lines []int
	}{
		{
			name:  "one flag",
			src:   "<?php\necho json_encode($a, JSON_PRETTY_PRINT);\n",
			lines: []int{2},
		},
		{
			name:  "several on one line",
			src:   "<?php\necho json_encode($a, JSON_HEX_TAG | JSON_HEX_AMP);\n",
			lines: []int{2, 2},
		},
		{
			name:  "no flags is clean",
			src:   "<?php\necho json_encode($a);\n",
			lines: nil,
		},
		{
			// The name inside a string is not a use, so a script probing for
			// the constant reads as written.
			name:  "a string is not a use",
			src:   "<?php\nvar_dump(defined(\"JSON_PRETTY_PRINT\"));\n$s = \"JSON_HEX_TAG\";\n",
			lines: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diags, err := lint.File("probe.php", test.src)
			if err != nil {
				t.Fatal(err)
			}
			var got []int
			for _, d := range diags {
				if !strings.Contains(d.Message, "the JSON encoding is not") {
					continue
				}
				got = append(got, d.Line)
			}
			if len(got) != len(test.lines) {
				t.Fatalf("lines = %v, want %v", got, test.lines)
			}
			for i := range got {
				if got[i] != test.lines[i] {
					t.Fatalf("lines = %v, want %v", got, test.lines)
				}
			}
		})
	}
}

// A call to a function nothing defines is the "call to undefined function"
// error waiting for the line to run; the linter reports it first. A name the
// file declares (at any nesting), a registered binding, or one the source
// guards with function_exists lints clean.
func TestFileReportsUndefinedFunctions(t *testing.T) {
	src := `<?php
echo strlen("known binding"), "\n";
echo hash_hmac("nothing registers this");
declared();
conditional();
if (function_exists("imagetypes")) {
	imagetypes();
}
function declared() { return 1; }
if (true) {
	function conditional() { return 2; }
}
$sorter = function ($rows) {
	return date("Y");
};
`
	diags, err := lint.File("funcs.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`funcs.php:3: call to undefined function hash_hmac()`,
		`funcs.php:14: call to undefined function date()`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

// A class reference nothing declares — a `new`, a static call, property or
// constant — is reported with the message the runtime would raise. Classes
// declared in the file, registered host classes, anonymous classes and the
// contextual self/static names lint clean.
func TestFileReportsUndefinedClasses(t *testing.T) {
	src := `<?php
class Declared {
	public static function make() {
		return new self();
	}
}
$a = new Declared();
$b = new Exception("registered host class");
$c = new ReflectionClass("nope");
ReflectionMethod::export();
$anon = new class {
	function touch() {}
};
echo Missing::NAME;
echo Missing::class, "\n";
$d = Missing::$prop;
`
	diags, err := lint.File("classes.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`classes.php:9: new: undefined class "ReflectionClass"`,
		`classes.php:10: static call ReflectionMethod::export(): unknown class`,
		`classes.php:14: class constant Missing::NAME: unknown class`,
		`classes.php:16: static property Missing::$prop: unknown class`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

// A magic method other than __construct and __invoke is never called
// implicitly (docs/design.md), so a class relying on one holds dead code that
// looks load-bearing. The two that run lint clean.
func TestFileReportsMagicMethods(t *testing.T) {
	src := `<?php
class Inject {
	public function __construct() {}
	public function __invoke() {}
	public function __call($name, $args) {
		return null;
	}
	public function __get($name) {
		return null;
	}
}
`
	diags, err := lint.File("magic.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`magic.php:5: magic method Inject::__call() is never called implicitly: only __construct and __invoke run; declare an explicit method (docs/design.md)`,
		`magic.php:8: magic method Inject::__get() is never called implicitly: only __construct and __invoke run; declare an explicit method (docs/design.md)`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

// The reference markers parse and confer nothing (docs/design.md): `$a = &$b`
// binds by value and `function &f()` returns by value, in free functions,
// methods and closures alike. The foreach form, parameters and closure `use`
// captures really do carry semantics elsewhere and are not reported.
func TestFileReportsReferences(t *testing.T) {
	src := `<?php
$b = 2;
$a = &$b;
function &getRef() { return 1; }
class Box { public function &value() { return 7; } }
$f = function &() { return 1; };
foreach (array(1) as &$v) { $v = 2; }
$g = function () use (&$b) { return $b; };
function takesRef(&$x) { return $x; }
`
	diags, err := lint.File("refs.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`refs.php:3: reference & is a no-op: the value is bound by value, and a later write through one name is not seen through the other (docs/design.md)`,
		`refs.php:4: function &getRef() returns by value: the & is a no-op; return the value (docs/design.md)`,
		`refs.php:5: function &Box::value() returns by value: the & is a no-op; return the value (docs/design.md)`,
		`refs.php:6: function &() returns by value: the & is a no-op; return the value (docs/design.md)`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

// The abstract modifier has nothing to mean without inheritance: the class
// instantiates like any other and an abstract method's call returns null
// where PHP refuses to load the incomplete class. Both spellings are
// reported; a plain class with bodies lints clean.
func TestFileReportsAbstract(t *testing.T) {
	src := `<?php
abstract class Shape {
	abstract public function area();
	public function describe() {
		return "shape";
	}
}
class Circle {
	public function area() {
		return 1;
	}
}
`
	diags, err := lint.File("shape.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`shape.php:2: abstract is a no-op: Shape can be instantiated; declare an interface for the contract (docs/design.md)`,
		`shape.php:3: abstract method Shape::area() is a no-op: it has no body and a call returns null; declare the body (docs/design.md)`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}
