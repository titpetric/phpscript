package test

import (
	"testing"
	"time"

	"github.com/titpetric/phpscript/tests"
)

// TestMatrixRowLabel covers what a cell shows for a row. The label is the
// basename the table prints; a row built without one falls back to the display
// path and a cell is never blank.
func TestMatrixRowLabel(t *testing.T) {
	t.Parallel()
	labelled := matrixRow{DisplayPath: "tests/fixtures/arrays/a.phpt", Label: "a.phpt"}
	if got := labelled.label(); got != "a.phpt" {
		t.Errorf("label() = %q, want the basename", got)
	}

	bare := matrixRow{DisplayPath: "tests/fixtures/arrays/a.phpt"}
	if got := bare.label(); got != "tests/fixtures/arrays/a.phpt" {
		t.Errorf("label() = %q, want the display path", got)
	}

	if got := (matrixRow{}).label(); got != "" {
		t.Errorf("label() = %q for a row naming nothing", got)
	}
}

// TestMatrixRowFailed covers the verdict a row reports to the run. Only a
// failure fails it: a skip is a runner the fixture opted out of or one the
// machine does not have, and counting either as a failure would fail every run
// on a machine with no php.
func TestMatrixRowFailed(t *testing.T) {
	t.Parallel()
	for name, row := range map[string]matrixRow{
		"every runner passed": {Cells: []matrixCell{
			{Runner: tests.RunnerFlatstack, Status: matrixPass},
			{Runner: tests.RunnerRuntime, Status: matrixPass},
		}},
		"a runner was skipped": {Cells: []matrixCell{
			{Runner: tests.RunnerFlatstack, Status: matrixPass},
			{Runner: tests.RunnerPHP, Status: matrixSkip, Reason: "php is not installed"},
		}},
		"every runner was skipped": {Cells: []matrixCell{
			{Runner: tests.RunnerPHP, Status: matrixSkip},
		}},
		"no runner ran": {},
	} {
		if row.Failed() {
			t.Errorf("%s: Failed() = true", name)
		}
	}

	failing := matrixRow{Cells: []matrixCell{
		{Runner: tests.RunnerFlatstack, Status: matrixPass},
		{Runner: tests.RunnerRuntime, Status: matrixFail, Reason: "output mismatch"},
		{Runner: tests.RunnerPHP, Status: matrixSkip},
	}}
	if !failing.Failed() {
		t.Error("Failed() = false for a row one runner failed")
	}
}

// TestEngineOrder covers the order every table and summary line prints the
// per-engine durations in, which is the run's own runner order and never the
// map's. A runner whose fixtures all opted out reports 0ms and stays in the
// report and a column does not appear and vanish between folders.
func TestEngineOrder(t *testing.T) {
	t.Parallel()
	runners := []tests.Runner{tests.RunnerFlatstack, tests.RunnerRuntime, tests.RunnerPHP}
	durations := map[tests.Runner]time.Duration{
		tests.RunnerPHP:       3 * time.Millisecond,
		tests.RunnerFlatstack: time.Millisecond,
	}

	engines := engineOrder(runners, durations)
	if len(engines) != len(runners) {
		t.Fatalf("engines = %d, want one per runner", len(engines))
	}
	for i, engine := range engines {
		if engine.Runner != runners[i] {
			t.Errorf("engine %d is %s, want %s", i, engine.Runner, runners[i])
		}
	}
	if engines[1].Duration != 0 {
		t.Errorf("the runner with no measured time reports %v, want 0", engines[1].Duration)
	}
	if engines[2].Duration != 3*time.Millisecond {
		t.Errorf("php reports %v, want 3ms", engines[2].Duration)
	}

	if got := engineOrder(nil, durations); len(got) != 0 {
		t.Errorf("engineOrder over no runners = %v, want nothing", got)
	}
}
