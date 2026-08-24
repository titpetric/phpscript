package flatstack_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
)

// TestFlatstackCompilesClosures covers the anonymous-function forms the compiler
// lowers. Every case asserts Supports before it checks the output, because a
// fallback to the interpreter would produce the same output and hide a compiler
// that quietly stopped handling the form.
func TestFlatstackCompilesClosures(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "comparator argument",
			source: `<?php $n = array(3, 1, 2); usort($n, function ($a, $b) { return $a - $b; }); echo implode(",", $n);`,
			want:   "1,2,3",
		},
		{
			name:   "comparator held in a variable",
			source: `<?php $c = function ($a, $b) { return strlen($a) - strlen($b); }; $w = array("ccc", "a", "bb"); usort($w, $c); echo implode(",", $w);`,
			want:   "a,bb,ccc",
		},
		{
			// The capture is read where the closure value is created, so a
			// later write to the captured variable is not visible to it.
			name:   "use captures a snapshot",
			source: `<?php $m = 3; $f = function ($x) use ($m) { return $x * $m; }; $m = 10; echo implode(",", array_map($f, array(1, 2)));`,
			want:   "3,6",
		},
		{
			name:   "parameter shadows a capture of the same name",
			source: `<?php $x = "outer"; echo call_user_func(function ($x) { return $x; }, "inner");`,
			want:   "inner",
		},
		{
			name:   "an omitted argument binds null",
			source: `<?php echo var_export(call_user_func(function ($a, $b) { return $b; }, 1), true);`,
			want:   "NULL",
		},
		{
			name:   "a closure written in a method carries $this",
			source: `<?php class Box { public $n = "box"; function tag($items) { return array_map(function ($i) { return $this->n . ":" . $i; }, $items); } } $b = new Box; echo implode(",", $b->tag(array("a", "b")));`,
			want:   "box:a,box:b",
		},
		{
			name:   "static closure",
			source: `<?php echo call_user_func(static function () { return "static"; });`,
			want:   "static",
		},
		{
			name:   "a closure returning a closure",
			source: `<?php $add = function ($x) { return function ($y) use ($x) { return $x + $y; }; }; echo implode(",", array_map(call_user_func($add, 10), array(1, 2)));`,
			want:   "11,12",
		},
		{
			name:   "control flow and a call in the body",
			source: `<?php function twice($v) { return $v * 2; } echo call_user_func(function ($limit) { $sum = 0; for ($i = 0; $i < $limit; $i++) { $sum += twice($i); } return $sum; }, 4);`,
			want:   "12",
		},
		{
			// A closure created inside a try is jumped over without leaving the
			// handler the try armed, and one that throws is caught by the try
			// the call was made from.
			name:   "created and throwing inside a try",
			source: `<?php try { $f = function () { throw new Exception("boom"); }; echo implode(",", array_map($f, array(1))); } catch (Exception $e) { echo "caught:", $e->getMessage(); }`,
			want:   "caught:boom",
		},
		{
			// usort() has the caller's variables bound while it runs its
			// comparator. The comparator installs its own binding for the calls
			// it makes, and has to leave the caller's in place on the way out.
			name:   "a comparator does not overwrite the caller's variables",
			source: `<?php $a = 1; $b = 2; $n = array(3, 1, 2); usort($n, function ($a, $b) { return $a - $b; }); echo $a, $b, implode(",", $n);`,
			want:   "121,2,3",
		},
		{
			// preg_match writes an output parameter through the frame the call
			// was made from, which for this one is the closure's own.
			name:   "an output parameter inside a closure body",
			source: `<?php $keep = "kept"; $hits = array_map(function ($s) { preg_match('/([0-9])/', $s, $m); return $m[1]; }, array("a1", "b2")); echo implode(",", $hits), $keep;`,
			want:   "1,2kept",
		},
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
			stdlib.Register(runtime)
			if err := runtime.Run(program); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

// TestFlatstackRejectsClosureForms pins the closure forms that keep going to the
// interpreter. Each one needs something a compiled frame does not have: a
// reference cell for the enclosing variable, an expression evaluated on the
// missing-argument path, or the caller's scope itself.
func TestFlatstackRejectsClosureForms(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "by-reference capture",
			source: `<?php $n = 0; $f = function () use (&$n) { $n++; };`,
			want:   "unsupported by-reference capture use (&$n)",
		},
		{
			name:   "parameter default",
			source: `<?php $f = function ($a = 1) { return $a; };`,
			want:   "unsupported closure parameter default",
		},
		{
			name:   "by-reference parameter",
			source: `<?php $f = function (&$a) { $a = 1; };`,
			want:   "unsupported by-reference closure parameter",
		},
		{
			name:   "variadic parameter",
			source: `<?php $f = function (...$rest) { return $rest; };`,
			want:   "unsupported variadic closure parameter",
		},
		{
			name:   "invoking a callable held in a value",
			source: `<?php $f = function () { return 1; }; echo $f();`,
			want:   "unsupported expression *model.Invoke",
		},
		{
			name:   "defer",
			source: `<?php defer(function () { echo "late"; });`,
			want:   "unsupported defer() registers on the calling frame",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			err = flatstack.Supports(program)
			if err == nil {
				t.Fatal("expected the form to be rejected")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want one containing %q", err, test.want)
			}
		})
	}
}
