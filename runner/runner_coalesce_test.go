package runner

import (
	"strings"
	"testing"
)

// TestCoalesceAssignResolvesTheTargetOnce covers `??=` over the targets whose
// base or key runs script code: a call in the index, an increment, the
// receiver of a property and a property name an expression spells. php runs
// each once, whether the store happens or not.
func TestCoalesceAssignResolvesTheTargetOnce(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	program, err := rt.Load(`<?php
class Calls { public static $n = 0; }
function key_once() { Calls::$n += 1; return "k"; }
$held = ["k" => "held"];
$held[key_once()] ??= "stored";
$fresh = [];
$fresh[key_once()] ??= "stored";
$i = 1;
$t = [1 => "x"];
$t[$i++] ??= "first";
$t[$i++] ??= "second";
class Side { public $p = null; public static $one = null; }
function side() { Calls::$n += 1; if (Side::$one === null) { Side::$one = new Side; } return Side::$one; }
side()->p ??= "filled";
$name = "p";
$dyn = new Side;
$dyn->$name ??= "dyn";
echo $held["k"], "|", $fresh["k"], "|", $t[1], $t[2], "|", $i, "|", side()->p, "|", $dyn->p, "|", Calls::$n;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Three calls: one per key_once and one for the receiver that stores. The
	// side() in the echo is the fourth.
	if want := "held|stored|xsecond|3|filled|dyn|4"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// TestCoalesceAssignLeavesAStringOffsetAlone covers the target php does not
// write: a byte the string already holds keeps its value. The read runs through
// helperIndex, where the array path an assignment takes writes.
func TestCoalesceAssignLeavesAStringOffsetAlone(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	program, err := rt.Load(`<?php
$s = "abc";
$s[0] ??= "z";
$o = new stdClass;
$o->set = "kept";
$o->set ??= "replaced";
$o->fresh ??= "written";
echo $s, "|", $o->set, "|", $o->fresh;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "abc|kept|written"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
