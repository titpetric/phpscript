package stdlib

import (
	"slices"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestAreas covers the introspection of the mounted runtime: every default area
// is named under Runtime\PS or Runtime\PX, the PS areas come first, a name has one
// owner, and the opt-in Go area lists apart.
func TestAreas(t *testing.T) {
	infos := Areas(All)
	if len(infos) < 10 {
		t.Fatalf("only %d areas", len(infos))
	}
	seenPX := false
	for _, info := range infos {
		if info.OptIn {
			continue
		}
		isPX := runner.InNamespace(info.Name, runner.NamespacePX)
		if !isPX && !runner.InNamespace(info.Name, runner.NamespacePS) {
			t.Errorf("%s is under neither Runtime\\PS nor Runtime\\PX", info.Name)
		}
		if seenPX && !isPX {
			t.Errorf("%s installs after a Runtime\\PX area", info.Name)
		}
		seenPX = seenPX || isPX
	}
	owners := Owners(infos)
	if owners.Funcs["strlen"] != `Runtime\PS\Core\Strings` {
		t.Errorf("strlen owner = %q", owners.Funcs["strlen"])
	}
	var goArea *AreaInfo
	for i := range infos {
		if infos[i].OptIn && infos[i].Name == runner.NamespaceGo {
			goArea = &infos[i]
		}
	}
	if goArea == nil || !slices.Contains(goArea.Funcs, `Runtime\Go\make`) {
		t.Errorf("Runtime\\Go area missing or without make: %+v", goArea)
	}
}

// TestAreasSelected covers the namespace filter: a host that names Runtime\PS\Core
// gets no Runtime\PX area.
func TestAreasSelected(t *testing.T) {
	for _, info := range Areas(All, `Runtime\PS\Core`) {
		if info.OptIn || info.Name == ErrorsArea || info.Name == ExceptionsArea {
			continue
		}
		if !runner.InNamespace(info.Name, `Runtime\PS\Core`) {
			t.Errorf("%s is outside the selection", info.Name)
		}
	}
}

// TestQualifiedAreaCall covers a call written through its area: the area name
// stands in front of the function and reaches the same binding as the bare name.
func TestQualifiedAreaCall(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	Register(rt)
	program, err := rt.Load(`<?php
echo \Runtime\PS\Core\Strings\strlen("four"), "|", Runtime\PS\Core\Strings\STRTOUPPER("a"), "|";
var_dump(function_exists('Runtime\PS\Core\Strings\strlen'), function_exists('Runtime\PS\Core\Arrays\strlen'));`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if want := "4|A|bool(true)\nbool(false)\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// TestQualifiedAreaClass covers class_exists and new through a binding area,
// over the three spellings AreaName produces and one name no area installs.
func TestQualifiedAreaClass(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	Register(rt)
	program, err := rt.Load(`<?php
var_dump(class_exists('Runtime\PS\Database'), class_exists('Runtime\PS\Database\Migrate'));
var_dump(class_exists('runtime\ps\exceptions\RuntimeException'), class_exists('Runtime\PS\Errors\TypeError'));
var_dump(class_exists('Runtime\PS\Database\Missing'));
$error = new \Runtime\PS\Errors\TypeError("qualified", 3);
echo get_class($error), "|", $error->getMessage(), "|", $error->getCode(), "\n";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	want := "bool(true)\nbool(true)\nbool(true)\nbool(true)\nbool(false)\nTypeError|qualified|3\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}
