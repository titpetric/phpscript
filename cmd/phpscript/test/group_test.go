package test

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/titpetric/phpscript/tests"
)

func TestMapFixturesRunsConcurrentlyWithSerialBarrier(t *testing.T) {
	fixtures := []*tests.Fixture{{Name: "a"}, {Name: "b"}, {Name: "serial", Serial: true}, {Name: "c"}, {Name: "d"}}
	var active, peak, serialActive atomic.Int64

	results := mapFixtures(fixtures, 4, func(_, i int, fx *tests.Fixture) string {
		current := active.Add(1)
		defer active.Add(-1)
		for current > peak.Load() && !peak.CompareAndSwap(peak.Load(), current) {
		}
		if fx.Serial {
			serialActive.Store(current)
		}
		time.Sleep(10 * time.Millisecond)
		return fx.Name
	})

	if got := strings.Join(results, ","); got != "a,b,serial,c,d" {
		t.Fatalf("result order = %q", got)
	}
	if peak.Load() < 2 {
		t.Fatalf("peak concurrency = %d, want at least 2", peak.Load())
	}
	if serialActive.Load() != 1 {
		t.Fatalf("serial fixture active count = %d, want 1", serialActive.Load())
	}
}

// TestMapFixturesSerialRun covers a run without --parallel: the fixtures
// execute in order on the caller's goroutine. mapFixtures numbers each worker
// zero; a worker-scoped cache is a single cache for the run.
func TestMapFixturesSerialRun(t *testing.T) {
	fixtures := []*tests.Fixture{{Name: "a"}, {Name: "b"}, {Name: "c"}}

	for _, parallel := range []int{0, 1} {
		var order []string
		var workers []int
		results := mapFixtures(fixtures, parallel, func(worker, i int, fx *tests.Fixture) string {
			order = append(order, fx.Name)
			workers = append(workers, worker)
			return fx.Name
		})
		if got := strings.Join(results, ","); got != "a,b,c" {
			t.Errorf("parallel %d: results = %q, want a,b,c", parallel, got)
		}
		if got := strings.Join(order, ","); got != "a,b,c" {
			t.Errorf("parallel %d: ran in order %q, want a,b,c", parallel, got)
		}
		for _, worker := range workers {
			if worker != 0 {
				t.Errorf("parallel %d: a fixture ran on worker %d, want the one worker", parallel, worker)
			}
		}
	}

	// A single fixture stays single whatever --parallel says. mapFixtures
	// skips the batching and does not enter it with one job.
	results := mapFixtures(fixtures[:1], 8, func(worker int, _ int, fx *tests.Fixture) string {
		if worker != 0 {
			t.Errorf("the only fixture ran on worker %d", worker)
		}
		return fx.Name
	})
	if len(results) != 1 || results[0] != "a" {
		t.Errorf("results = %v, want the one fixture", results)
	}
}

// TestGroupFixtures covers the bucketing the per-area tables print from.
// groupFixtures keeps discovery order between folders and within a folder. Two
// runs over the same tree print the same report. A diff of two reports reflects
// the results.
func TestGroupFixtures(t *testing.T) {
	fixtures := []*tests.Fixture{
		{Name: "b"}, {Name: "a"}, {Name: "s"}, {Name: "root"},
	}
	paths := []string{
		"tests/fixtures/arrays/b.phpt",
		"tests/fixtures/arrays/a.phpt",
		"tests/fixtures/strings/s.phpt",
		"root.phpt",
	}

	groups := groupFixtures(fixtures, paths)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want one per folder", len(groups))
	}

	first := groups[0]
	if first.Dir != "tests/fixtures/arrays" {
		t.Errorf("the first group is %q, want the folder discovery reached first", first.Dir)
	}
	if got := strings.Join(first.Labels, ","); got != "b.phpt,a.phpt" {
		t.Errorf("labels = %q, want discovery order inside the folder", got)
	}
	if got := strings.Join(first.Paths, ","); got != "tests/fixtures/arrays/b.phpt,tests/fixtures/arrays/a.phpt" {
		t.Errorf("paths = %q, want the display paths as they were given", got)
	}
	if len(first.Fixtures) != 2 || first.Fixtures[0].Name != "b" {
		t.Errorf("fixtures = %+v, want the two of that folder in order", first.Fixtures)
	}

	// A fixture in the run directory forms a separate group, named as the
	// table prints it.
	last := groups[len(groups)-1]
	if last.Dir != "." {
		t.Errorf("the run directory group is %q, want \".\"", last.Dir)
	}
	if got := strings.Join(last.Labels, ","); got != "root.phpt" {
		t.Errorf("labels = %q, want root.phpt", got)
	}
}
