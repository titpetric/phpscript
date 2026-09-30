package flatstack_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
)

// TestFlatstackCompilesConstructs pins the constructs that used to drop a whole
// program back to the compatibility interpreter. A fixture cannot tell a
// bytecode run from a fallback that happened to agree, so every case asserts
// Supports before it checks the output.
func TestFlatstackCompilesConstructs(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "self class constant",
			source: `<?php class A { const X = "x"; function get() { $v = self::X; return $v; } } $a = new A(); echo $a->get();`,
			want:   "x",
		},
		{
			name:   "static class constant",
			source: `<?php class A { const X = "x"; function get() { return static::X; } } $a = new A(); echo $a->get();`,
			want:   "x",
		},
		{
			// php rejects parent with no parent class; the interpreter's
			// resolveClassName collapses it to the current class, so this pins
			// the two backends to each other rather than to php.
			name:   "parent class constant resolves to the enclosing class",
			source: `<?php class A { const X = "x"; function get() { return parent::X; } } $a = new A(); echo $a->get();`,
			want:   "x",
		},
		{
			name:   "another class named at top level",
			source: `<?php class A { const X = "x"; } echo A::X;`,
			want:   "x",
		},
		{
			name:   "class constant referring to another class constant",
			source: `<?php class A { const X = "x"; const Y = self::X . "y"; function get() { return self::Y; } } $a = new A(); echo $a->get();`,
			want:   "xy",
		},
		{
			name:   "the class pseudo-constant",
			source: `<?php class A { function get() { return self::class; } } $a = new A(); echo $a->get();`,
			want:   "A",
		},
		{
			name:   "cast in a method body",
			source: `<?php class B { public $v = false; function set($x) { $this->v = (bool)$x; return $this->v; } } $b = new B(); echo $b->set(1) ? "true" : "false";`,
			want:   "true",
		},
		{
			name:   "scalar casts",
			source: `<?php echo (int)"12ab", ",", (float)"1.5", ",", (string)true, ",", (bool)"" ? "t" : "f";`,
			want:   "12,1.5,1,f",
		},
		{
			name:   "array cast",
			source: `<?php $a = (array)"x"; echo count($a), $a[0];`,
			want:   "1x",
		},
		{
			name:   "list into a property",
			source: `<?php class C { public $name = "start"; public $stack = array("a","b"); function pop() { list($this->name) = array_splice($this->stack, -1); return $this->name; } } $c = new C(); echo $c->pop();`,
			want:   "b",
		},
		{
			name:   "foreach into a property",
			source: `<?php class D { public $last = ""; function run() { foreach (array("a","b") as $this->last) {} return $this->last; } } $d = new D(); echo $d->run();`,
			want:   "b",
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

// TestFlatstackClassConstOutsideClass pins the message a contextual class name
// used outside a class body produces. The compiler leaves the name alone when
// there is nothing to resolve it to, so the host reports it exactly as the
// interpreter does instead of inventing a class.
func TestFlatstackClassConstOutsideClass(t *testing.T) {
	program, err := parser.Parse(`<?php class A { const X = 1; } echo self::X;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatalf("expected native bytecode support: %v", err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	stdlib.Register(runtime)
	err = runtime.Run(program)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	const want = "class constant self::X: unknown class"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}

// TestFlatstackDestructuringTargetKind pins the wording of a rejected
// destructuring target: storeTop serves both foreach and list(), and used to
// report a foreach for either one.
func TestFlatstackDestructuringTargetKind(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "list reports list()",
			source: `<?php class E { static $s; } list(E::$s) = array(1);`,
			want:   "unsupported list() target *model.StaticProp",
		},
		{
			name:   "foreach reports foreach",
			source: `<?php class E { static $s; } foreach (array(1) as E::$s) {}`,
			want:   "unsupported foreach target *model.StaticProp",
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
				t.Fatal("expected the target to be rejected")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want one containing %q", err, test.want)
			}
		})
	}
}
