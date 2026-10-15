package core_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestGetClass covers the name get_class answers with. An interpreted object
// reports its declared class, and a missing or null argument is
// false, which is php's answer for the call that names nothing.
func TestGetClass(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
class Site {}
$s = new Site;
var_dump(get_class($s), get_class(null));`)

	want := "string(4) \"Site\"\nbool(false)\n"
	if got != want {
		t.Errorf("get_class = %q, want %q", got, want)
	}
}

// TestGetClassOfAHostValue pins a divergence the field doc states: a value a Go
// constructor returns reports its Go type name, never the constructor's
// registered name. The two are usually the same word; where a
// host registers "Storage" for a Go type called storage they are not, and a
// script comparing get_class($x) === "Storage" reads false.
func TestGetClassOfAHostValue(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.RegisterConstructor("Holder", func() any { return &holder{Name: "held"} })

	program, err := rt.Load(`<?php $h = new Holder; echo get_class($h);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if want := "holder"; out.String() != want {
		t.Errorf("get_class of a host value = %q, want the Go type name %q", out.String(), want)
	}
}

// TestNoParentClass is the executable form of a design decision: phpscript has
// no inheritance, so no class has a parent to report and no object is an
// instance of a subclass. A guard written as `is_subclass_of($o, "Exception")
// || get_class($o) === "Exception"` falls through to the name comparison, which
// is the check that answers here. If this fails, read docs/design.md.
func TestNoParentClass(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
class Site {}
$s = new Site;
var_dump(get_parent_class($s), is_subclass_of($s, "Site"), get_class($s) === "Site");`)

	want := "bool(false)\nbool(false)\nbool(true)\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestGetObjectVars covers the properties an object reads back, in the order it
// reads them. A property named for its digits is an integer key once it is an
// array entry, so `(object) array("a", "b")` reads back as a list and a
// foreach over the result counts from zero.
func TestGetObjectVars(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
class Site { public $greeting = "hei"; }
$s = new Site;
$s->added = 1;
var_dump(get_object_vars($s));
var_dump(get_object_vars((object) array("a", "b")));
var_dump(get_object_vars("Site"), get_object_vars(42));`)

	want := "array(2) {\n" +
		"  [\"greeting\"]=>\n" +
		"  string(3) \"hei\"\n" +
		"  [\"added\"]=>\n" +
		"  int(1)\n" +
		"}\n" +
		"array(2) {\n" +
		"  [0]=>\n" +
		"  string(1) \"a\"\n" +
		"  [1]=>\n" +
		"  string(1) \"b\"\n" +
		"}\n" +
		"array(0) {\n}\n" +
		"array(0) {\n}\n"
	if got != want {
		t.Errorf("get_object_vars =\n%s\nwant:\n%s", got, want)
	}
}

// TestMethodExists covers php's case-insensitive method lookup, reached through
// an object or through a class name, both of which php accepts.
func TestMethodExists(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
class Site { public function hello() { return "hi"; } }
$s = new Site;
var_dump(method_exists($s, "hello"), method_exists($s, "HELLO"), method_exists($s, "nope"));
var_dump(method_exists("Site", "hello"), method_exists("Nope", "hello"));`)

	want := "bool(true)\nbool(true)\nbool(false)\nbool(true)\nbool(false)\n"
	if got != want {
		t.Errorf("method_exists = %q, want %q", got, want)
	}
}

// TestPropertyExists covers a property set on the instance and one the class
// declared, both of which answer true.
//
// A class name answers false, which php does not: there property_exists("Site",
// "greeting") is true. The declaration is reachable only through an instance
// here, so the call that names a class has nothing to read.
func TestPropertyExists(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
class Site { public $greeting = "hei"; }
$s = new Site;
$s->added = 1;
var_dump(property_exists($s, "greeting"), property_exists($s, "added"), property_exists($s, "nope"));
var_dump(property_exists("Site", "greeting"));`)

	want := "bool(true)\nbool(true)\nbool(false)\nbool(false)\n"
	if got != want {
		t.Errorf("property_exists = %q, want %q", got, want)
	}
}

// TestSPLObjectIdentity covers the identity php exposes, which is opaque and
// stable for as long as the object is alive, and that is the only guarantee php
// makes either. Two objects differ, one object reads the same twice, and the
// hash is the id in the 32 hex digits php writes it in.
func TestSPLObjectIdentity(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
class Site {}
$a = new Site;
$b = new Site;
var_dump(spl_object_id($a) === spl_object_id($a));
var_dump(spl_object_id($a) === spl_object_id($b));
var_dump(spl_object_hash($a) === spl_object_hash($a));
var_dump(spl_object_hash($a) === spl_object_hash($b));
var_dump(strlen(spl_object_hash($a)));`)

	want := "bool(true)\nbool(false)\nbool(true)\nbool(false)\nint(32)\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
