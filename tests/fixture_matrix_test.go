package tests

import (
	"testing"
)

// TestFixtureRunners covers the three states a runner opt-out carries. The
// fields are pointers so an absent one is distinguishable from a false one: a
// fixture declaring nothing about a runner runs on it. The default runtime has
// no field at all, because it defines the expected output the other two
// columns compare against.
func TestFixtureRunners(t *testing.T) {
	var absent FixtureRunners
	if absent.Flatstack != nil || absent.PHP != nil {
		t.Errorf("a zero FixtureRunners = %+v, want neither field set", absent)
	}

	off := false
	fx := &Fixture{Runner: FixtureRunners{Flatstack: &off}}
	if fx.Runs(RunnerFlatstack) {
		t.Error("Runs(flatstack) = true for a fixture that opted out")
	}
	if !fx.Runs(RunnerPHP) {
		t.Error("Runs(php) = false for a fixture declaring nothing about php")
	}
	if !fx.Runs(RunnerRuntime) {
		t.Error("Runs(runtime) = false; the runtime defines the expected output")
	}
}

func TestFixtureRunsMetadata(t *testing.T) {
	cases := []struct {
		name      string
		metadata  string
		flatstack bool
		php       bool
	}{
		{"default", "", true, true},
		{"php opted out", "runner:\n  php: false\n", true, false},
		{"flatstack opted out", "runner:\n  flatstack: false\n", false, true},
		{"both opted in", "runner:\n  flatstack: true\n  php: true\n", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "name: metadata\ndescription: runner metadata\n" + c.metadata + "---\n<?php echo \"ok\";\n---\nok"
			fx, err := ParseFixture([]byte(src), "metadata.phpt")
			if err != nil {
				t.Fatal(err)
			}
			if got := fx.Runs(RunnerFlatstack); got != c.flatstack {
				t.Errorf("Runs(flatstack) = %v, want %v", got, c.flatstack)
			}
			if got := fx.Runs(RunnerPHP); got != c.php {
				t.Errorf("Runs(php) = %v, want %v", got, c.php)
			}
			// The runtime defines the expected output, so nothing opts out
			// of it.
			if !fx.Runs(RunnerRuntime) {
				t.Error("Runs(runtime) = false, want true")
			}
		})
	}
}
