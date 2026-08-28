package flatstack_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
)

// TestFlatstackRejectsAnonymousClass pins the anonymous class as a form the
// compiler declines. The bytecode carries a class name where an anonymous class
// carries a declaration, so the whole program goes to the interpreter instead;
// this asserts the rejection rather than the output, because a fixture cannot
// tell a fallback from a compiled run that agreed with it.
func TestFlatstackRejectsAnonymousClass(t *testing.T) {
	program, err := parser.Parse(`<?php $o = new class { public function f() { return 1; } }; echo $o->f();`)
	if err != nil {
		t.Fatal(err)
	}
	err = flatstack.Supports(program)
	if err == nil {
		t.Fatal("expected an anonymous class to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported anonymous class") {
		t.Fatalf("error = %v, want one naming the anonymous class", err)
	}
}

// The rejection is a fallback, not a failure: the program still runs, and it
// produces what the interpreter produces.
func TestFlatstackFallsBackForAnonymousClass(t *testing.T) {
	program, err := parser.Parse(`<?php
interface Reader { public function get(); }
$o = new class implements Reader {
	public $value = "read";
	public function get() { return $this->value; }
};
echo $o->get(), $o instanceof Reader ? ":Reader" : ":no";
`)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	stdlib.Register(runtime)
	if err := runtime.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := output.String(); got != "read:Reader" {
		t.Fatalf("output = %q, want %q", got, "read:Reader")
	}
}
