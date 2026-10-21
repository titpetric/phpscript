package lint_test

import (
	"testing"

	"github.com/titpetric/phpscript/lint"
)

// docs/design.md documents global and extends as won't-builds that parse and
// do nothing, so the linter is where a port hears about them before the
// behaviour goes wrong at a distance.
func TestFileReportsGlobalAndExtends(t *testing.T) {
	t.Parallel()
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
// follow, so the interface form lints clean where the class form reports.
func TestFileAcceptsInterfaceExtends(t *testing.T) {
	t.Parallel()
	src := "<?php\ninterface Reader {\n\tfunction get($key);\n}\ninterface Store extends Reader {\n\tfunction put($key, $value);\n}\n"
	diags, err := lint.File("iface.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %v, want none", diags)
	}
}

// A magic method other than __construct and __invoke is never called
// implicitly (docs/design.md), so a class relying on one holds dead code that
// reads as if something depended on it. The two that run lint clean.
func TestFileReportsMagicMethods(t *testing.T) {
	t.Parallel()
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

// The abstract modifier has nothing to mean without inheritance: the class
// instantiates like any other and an abstract method's call returns null
// where PHP declines the incomplete class. The linter reports both
// spellings; a plain class with bodies lints clean.
func TestFileReportsAbstract(t *testing.T) {
	t.Parallel()
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
