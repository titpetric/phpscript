package runner_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

func runSource(t *testing.T, source string) string {
	t.Helper()
	program, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// A value a host binding produced is an object already. The `(object)` cast has
// to hand it back rather than fold it into a stdClass with a `scalar` property,
// which is what would happen if the collection test caught it or the scalar
// branch did. Exception is the binding every runtime has.
func TestObjectCastKeepsHostObjects(t *testing.T) {
	got := runSource(t, `<?php
$e = new Exception("boom");
$o = (object) $e;
echo get_class($o), "|", $o->getMessage(), "|", ($o === $e ? "same" : "different");
`)
	if want := "Exception|boom|same"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// An *model.Array is a pointer to a struct and would answer a "looks like an
// object" test, so the collection branch has to be tried first or `(object)` on
// an array becomes a no-op.
func TestObjectCastConvertsCollections(t *testing.T) {
	got := runSource(t, `<?php
$o = (object) array("host" => "localhost", "port" => 8080);
echo get_class($o), "|", $o->host, "|", $o->port, "|";
$fromList = (object) explode(",", "a,b");
echo json_encode($fromList);
`)
	if want := `stdClass|localhost|8080|{"0":"a","1":"b"}`; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// The scalar forms, and the round trip back through (array).
func TestObjectCastScalarsAndRoundTrip(t *testing.T) {
	got := runSource(t, `<?php
echo ((object) "text")->scalar, "|";
echo ((object) 5)->scalar, "|";
echo count(get_object_vars((object) null)), "|";
$back = (array) (object) array("a", "b");
var_dump(array_keys($back));
`)
	want := "text|5|0|array(2) {\n  [0]=>\n  int(0)\n  [1]=>\n  int(1)\n}\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
