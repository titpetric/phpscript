package runner

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
)

// TestPointer_Get covers the read a `*$p` makes.
func TestPointer_Get(t *testing.T) {
	t.Parallel()
	value := int64(7)
	p := &Pointer{get: func() (any, error) { return value, nil }}
	if got, err := p.Get(); err != nil || got != int64(7) {
		t.Fatalf("Get() = %#v, %v, want 7", got, err)
	}
}

// TestPointer_Set covers the write a `*$p = v` makes.
func TestPointer_Set(t *testing.T) {
	t.Parallel()
	var value any = int64(1)
	p := &Pointer{
		get: func() (any, error) { return value, nil },
		set: func(v any) error { value = v; return nil },
	}
	if err := p.Set(int64(2)); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if value != int64(2) {
		t.Fatalf("the addressed value reads %#v, want 2", value)
	}
}

// TestGoPointerOperators covers `&` and `*` through the parser and both
// engines. The pointer shares the variable, and an assignment through it
// writes the variable. A copy does not.
func TestGoPointerOperators(t *testing.T) {
	t.Parallel()
	src := `<?php
$a = 1;
$p = &$a;
*$p = 2;
echo $a, "|", *$p, "\n";
$b = $a;
$b = 5;
echo $a, "|", $b, "\n";
$list = [1, 2];
$q = &$list[1];
*$q = 9;
*$q += 1;
echo $list[1], "|", *$q, "\n";
`
	prog, err := parser.ParseFileOptions("main.php", src, parser.Options{GoRuntime: true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := New(&out, Options{Runtime: "go"})
	rt.UpdateFilename("main.php")
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "2|2\n2|5\n10|10\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// TestGoPointerProperty covers `&$obj->prop`: a pointer to a class property
// writes the property, as go's `&s.f` writes the field.
func TestGoPointerProperty(t *testing.T) {
	t.Parallel()
	src := `<?php
class User {
	public $age = 1;
}
$u = new User;
$p = &$u->age;
*$p = 2;
echo $u->age, "|", *$p, "\n";
`
	prog, err := parser.ParseFileOptions("main.php", src, parser.Options{GoRuntime: true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := New(&out, Options{Runtime: "go"})
	rt.UpdateFilename("main.php")
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "2|2\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// TestReadGoField covers a field of a binding's native struct.
func TestReadGoField(t *testing.T) {
	t.Parallel()
	type row struct{ Count int64 }
	if got, err := readGoField(row{Count: 3}, "count"); err != nil || got != int64(3) {
		t.Fatalf("readGoField = %#v, %v, want 3", got, err)
	}
	if _, err := readGoField(int64(1), "count"); err == nil {
		t.Fatal("a scalar answered a field read")
	}
}

// TestGoPointerNative covers the dereference of a binding's own Go pointer,
// which the type hints pass around as *T.
func TestGoPointerNative(t *testing.T) {
	t.Parallel()
	held := int64(3)
	if got, err := derefValue(&held); err != nil || got != int64(3) {
		t.Fatalf("derefValue = %#v, %v, want 3", got, err)
	}
	if err := writePointer(&held, func(any) (any, error) { return int64(4), nil }); err != nil {
		t.Fatalf("writePointer: %v", err)
	}
	if held != 4 {
		t.Fatalf("the pointer's value reads %d, want 4", held)
	}
	if _, err := derefValue(int64(1)); err == nil {
		t.Fatal("a dereference of a non-pointer answered no error")
	}
}
