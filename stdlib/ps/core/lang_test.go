package core_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestGetType covers the names gettype() answers with: PHP's original
// spellings; get_debug_type() uses the modern ones. "integer" and "double"
// where the value model says int and float, "boolean" where it says bool, NULL
// alone in capitals. The modern spellings pass every eyeball and fail every
// `gettype($x) === "integer"`.
func TestGetType(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
echo gettype(1), "|", gettype(1.5), "|", gettype("a"), "|", gettype(true), "|",
	gettype(null), "|", gettype([]), "|", gettype(new stdClass);`)

	if want := "integer|double|string|boolean|NULL|array|object"; got != want {
		t.Errorf("gettype = %q, want %q", got, want)
	}
}

// TestIntval covers the two halves of intval. Given no base, or base 10, it
// matches the (int) cast. Any other base applies to a string argument alone;
// intval(12.9, 16) is 12, whatever the hex reading of the digits would be.
//
// An array is the case the cast does not own. PHP reads it as truthiness: an
// empty array is 0; a filled one is 1 whatever it holds.
func TestIntval(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
echo intval("42abc"), "|", intval("abc"), "|", intval("  12  "), "|", intval("1e3"), "|",
	intval(true), "|", intval(null), "|", intval(12.9), "|", intval(12.9, 16), "|",
	intval([]), "|", intval([1, 2]), "|", intval(["k" => "v"]);`)

	if want := "42|0|12|1000|1|0|12|12|0|1|1"; got != want {
		t.Errorf("intval = %q, want %q", got, want)
	}
}

// TestIntvalBase covers the C strtol reading for a base other than 10: leading
// whitespace, one optional sign, then digits of the base until a character
// outside the base.
//
// Base 0 detects the base from a 0x, 0b or 0 prefix, and 10 otherwise. The 0o
// octal prefix is not among them: strtol does not know it. intval("0o12", 8) is
// 0. A base outside 0 and 2-36 is strtol's EINVAL, which intval surfaces as zero
// and raises nothing, php 8.5 included.
func TestIntvalBase(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
echo intval("0x1A", 16), "|", intval("0x1A", 0), "|", intval("  -0x10", 0), "|",
	intval("0b101", 0), "|", intval("012", 0), "|", intval("12", 0), "|", intval("0", 0), "|",
	intval("0o12", 8), "|", intval("zz", 36), "|", intval("+42", 10), "|",
	intval("", 16), "|", intval("0x", 16), "|", intval("12", 1), "|", intval("12", 37);`)

	if want := "26|26|-16|5|10|12|0|0|1295|42|0|0|0|0"; got != want {
		t.Errorf("intval with a base = %q, want %q", got, want)
	}
}

// TestIntvalSaturates covers the int64 bounds. The absolute value accumulates
// unsigned; -9223372036854775808 still reads exactly. One digit past the bound
// saturates the way strtol does.
func TestIntvalSaturates(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
echo intval("9223372036854775807", 10), "|", intval("9223372036854775808", 10), "|",
	intval("-9223372036854775808", 10), "|", intval("99999999999999999999", 10);`)

	if want := "9223372036854775807|9223372036854775807|-9223372036854775808|9223372036854775807"; got != want {
		t.Errorf("intval at the bounds = %q, want %q", got, want)
	}
}

// TestCallUserFuncArray covers the array spelling of a dynamic call. The
// arguments arrive as the list the array holds. A callable taking three
// parameters receives three arguments from the array's entries.
func TestCallUserFuncArray(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
function join3($a, $b, $c) { return $a . $b . $c; }
echo call_user_func_array("join3", ["x", "y", "z"]);
echo "|";
echo call_user_func_array(function (...$args) { return count($args); }, [1, 2, 3, 4]);
echo "|";
echo call_user_func_array(function () { return "none"; }, []);`)

	if want := "xyz|4|none"; got != want {
		t.Errorf("call_user_func_array = %q, want %q", got, want)
	}
}

// TestGetTypeSeparatesAnArrayFromAStruct covers the check gettype and the
// reflection functions share. A *model.Array is a pointer to a struct in Go, an
// array in PHP; the collection test comes first. A value a Go binding returned
// is an object: a script constructs it with new and reads its properties the way
// it reads an interpreted object's.
func TestGetTypeSeparatesAnArrayFromAStruct(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.RegisterConstructor("Holder", func() any { return &holder{Name: "held"} })

	program, err := rt.Load(`<?php
$arr = [1, 2];
$obj = new Holder;
echo gettype($arr), "|", gettype($obj), "|", gettype($obj->name), "|", $obj->name;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	if want := "array|object|string|held"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// holder is a host-backed class: a Go value a constructor returns. A script
// reads properties from it, as from an interpreted object.
type holder struct {
	Name string
}
