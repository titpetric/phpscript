package runner

import (
	"strings"
	"testing"
)

// TestContainerVivifiesEachLevelOnce covers the levels an index assignment
// creates: a key or a receiver that runs script code runs once per level,
// where php counts one, and the created array goes back where it belongs.
func TestContainerVivifiesEachLevelOnce(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	program, err := rt.Load(`<?php
class Calls { public static $n = 0; }
function k($name) { Calls::$n += 1; return $name; }
$tree = [];
$tree[k("l1")][k("l2")] = "leaf";
$tree[k("l1")][k("l3")] ??= "second";
class Bag { public $rows = null; }
$bag = new Bag;
function f($b) { Calls::$n += 1; return $b; }
f($bag)->rows["k"] = 1;
f($bag)->rows["j"] ??= 2;
echo $tree["l1"]["l2"], $tree["l1"]["l3"], "|", $bag->rows["k"], $bag->rows["j"], "|", Calls::$n;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Six calls: two keys per $tree statement, one receiver per f() statement.
	if want := "leafsecond|12|6"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
