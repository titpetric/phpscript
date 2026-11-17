package tests

import (
	"testing"
)

// TestFixtureRunners covers the three states a runner opt-out holds. The
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
		{"go runtime", "runtime: go\n", true, false},
		{"go runtime with php opted in", "runtime: go\nrunner:\n  php: true\n", true, true},
		{"go runtime through options", "options:\n  runtime: go\n", true, false},
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
			// The runtime defines the expected output and nothing opts out
			// of it.
			if !fx.Runs(RunnerRuntime) {
				t.Error("Runs(runtime) = false, want true")
			}
		})
	}
}

// TestFixtureRuntimeMetadata covers the runtime key, its merge over options and
// the refusal of a name that is no mode.
func TestFixtureRuntimeMetadata(t *testing.T) {
	parse := func(metadata string) (*Fixture, error) {
		src := "name: runtime\ndescription: runtime metadata\n" + metadata + "---\n<?php echo \"ok\";\n---\nok"
		return ParseFixture([]byte(src), "runtime.phpt")
	}

	fx, err := parse("runtime: go\noptions:\n  runtime: phpscript\n")
	if err != nil {
		t.Fatal(err)
	}
	if !fx.runnerOptions().GoRuntime() {
		t.Errorf("runtime = %q, want the metadata key to win", fx.runnerOptions().Runtime)
	}

	if _, err := parse("runtime: php\n"); err == nil {
		t.Error("a runtime naming no mode parsed")
	}
}
