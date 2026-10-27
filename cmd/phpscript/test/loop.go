package test

import (
	"context"
	"runtime"
	"runtime/metrics"
	"sort"
	"time"

	"github.com/titpetric/phpscript/tests"
)

// fixtureMetrics is the cost of running a fixture, kept apart from the outcome
// so a matrix row can carry it too.
type fixtureMetrics struct {
	Runs        int
	Total       time.Duration
	P50         time.Duration
	P95         time.Duration
	P99         time.Duration
	AllocsPerOp uint64
	BytesPerOp  uint64
	GCRuns      uint32
}

// fixtureRun aggregates the outcome of one fixture over one or more runs.
// DisplayPath is the full path, which a failure line and the JSON report name;
// Label is the basename, which a per-folder table shows.
type fixtureRun struct {
	Result      *tests.TestResult
	DisplayPath string
	Label       string
	// Fixture is the fixture the run came from, carried so a table can read
	// the coverage collector installed on it. It stays nil for a run a test
	// assembles.
	Fixture *tests.Fixture
	fixtureMetrics
}

// label returns what a table cell should show for the run.
func (r *fixtureRun) label() string {
	if r.Label != "" {
		return r.Label
	}
	return r.DisplayPath
}

func runFixtureLoop(ctx context.Context, fx *tests.Fixture, r tests.Runner, opts Options) *fixtureRun {
	out := &fixtureRun{}
	hint := opts.Count
	if hint <= 0 {
		hint = 64
	}
	samples := make([]int64, 0, hint)

	// MemStats only when something reports it. ReadMemStats stops the world,
	// and this runs once per fixture per runner - 1368 pauses over a 342-file
	// matrix: for two numbers nothing prints without --profile.
	// The GC count comes from runtime/metrics either way, which does not stop
	// anything and the column stays.
	var before, after runtime.MemStats
	if opts.Profile {
		runtime.ReadMemStats(&before)
	}
	gcBefore := gcCycles()
	start := time.Now()
	for {
		opStart := time.Now()
		res := tests.RunFixtureOn(ctx, fx, r)
		samples = append(samples, time.Since(opStart).Nanoseconds())
		out.Runs++
		if out.Result == nil || (out.Result.Passed && !res.Passed) {
			out.Result = res
		}
		more := false
		if opts.Count > 0 && out.Runs < opts.Count {
			more = true
		}
		if opts.Time > 0 && time.Since(start) < opts.Time {
			more = true
		}
		if !more || ctx.Err() != nil {
			break
		}
	}
	out.Total = time.Since(start)
	out.GCRuns = uint32(gcCycles() - gcBefore)
	if opts.Profile {
		runtime.ReadMemStats(&after)
		// Per op, not per fixture: --count and --time run the fixture N times
		// inside the one pair of readings and the loop divides the totals by
		// the runs and never samples per iteration.
		n := uint64(out.Runs)
		out.AllocsPerOp = (after.Mallocs - before.Mallocs) / n
		out.BytesPerOp = (after.TotalAlloc - before.TotalAlloc) / n
	}
	if opts.Time > 0 && len(samples) > 0 {
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		out.P50 = time.Duration(percentileNs(samples, 50))
		out.P95 = time.Duration(percentileNs(samples, 95))
		out.P99 = time.Duration(percentileNs(samples, 99))
	}
	return out
}

func percentileNs(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p*len(sorted) + 99) / 100
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return sorted[idx-1]
}

func runFixtureSamples(ctx context.Context, fx *tests.Fixture, r tests.Runner, opts Options) []*fixtureRun {
	if opts.Count <= 0 || opts.Time <= 0 {
		return []*fixtureRun{runFixtureLoop(ctx, fx, r, opts)}
	}

	sampleOpts := opts
	sampleOpts.Count = 0
	runs := make([]*fixtureRun, 0, opts.Count)
	for range opts.Count {
		runs = append(runs, runFixtureLoop(ctx, fx, r, sampleOpts))
		if ctx.Err() != nil {
			break
		}
	}
	return runs
}

// gcCycles returns the number of completed GC cycles.
//
// runtime/metrics and not runtime.MemStats: NumGC is only reachable through
// ReadMemStats, which stops the world, and runFixtureLoop reads it twice per
// fixture per runner. The metrics reader takes a lock and copies a counter.
func gcCycles() uint64 {
	sample := [1]metrics.Sample{{Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(sample[:])
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}
