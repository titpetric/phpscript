package lint_test

import (
	"testing"

	"github.com/titpetric/phpscript/lint"
)

func TestFileReportsChainedAssignment(t *testing.T) {
	t.Parallel()
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

	// A chained statement whose value is a handle, or a name whose type the
	// source does not settle, draws one finding, whatever scope it sits in,
	// and a chain of three names draws one too and not one per link.
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

// The check leaves a chain that ends in a literal alone. A scalar is
// immutable and neither name can reach the other through it, and the parser
// splits an array literal into one allocation per name, with no value
// left to share. `$r['y'] = $r['m'] = '00'` is the shape this comes from.
func TestFileAcceptsChainedLiterals(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// The reference markers parse and confer nothing (docs/design.md): `$a = &$b`
// binds by value and `function &f()` returns by value, in free functions,
// methods and closures alike. The foreach form, parameters and closure `use`
// captures really do carry semantics elsewhere and are not reported.
func TestFileReportsReferences(t *testing.T) {
	t.Parallel()
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
