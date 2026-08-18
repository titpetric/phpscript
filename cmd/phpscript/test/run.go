package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/tests"
)

// Name is the command title.
const Name = "Run .phpt test fixtures"

// Options holds CLI flag options for the test command.
type Options struct {
	JSON       bool
	Matrix     bool
	Verbose    bool
	Count      int
	Time       time.Duration
	Profile    bool
	CPUProfile string
	MemProfile string
}

type jsonFixture struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Runner      string `json:"runner,omitempty"`
	Passed      bool   `json:"passed"`
	Skipped     bool   `json:"skipped,omitempty"`
	Runs        int    `json:"runs"`
	DurationNs  int64  `json:"duration_ns"`
	P50Ns       int64  `json:"p50_ns,omitempty"`
	P95Ns       int64  `json:"p95_ns,omitempty"`
	P99Ns       int64  `json:"p99_ns,omitempty"`
	AllocsPerOp uint64 `json:"allocs_per_op,omitempty"`
	BytesPerOp  uint64 `json:"bytes_per_op,omitempty"`
	GCRuns      uint32 `json:"gc_runs"`
	Failure     string `json:"failure_reason,omitempty"`
}

type jsonReport struct {
	Total   int           `json:"total"`
	Passed  int           `json:"passed"`
	Failed  int           `json:"failed"`
	Results []jsonFixture `json:"results"`
}

func NewCommand() *cli.Command {
	var opts Options
	return &cli.Command{
		Name:  "test",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&opts.JSON, "json", false, "Write machine-readable JSON to stdout")
			fs.BoolVar(&opts.Matrix, "matrix", false, "Run every fixture through all runtimes and report a matrix")
			fs.BoolVarP(&opts.Verbose, "verbose", "v", false, "Report the failure of each runtime below its fixture")
			fs.IntVarP(&opts.Count, "count", "c", 0, "Run each test N times; with --time, produce N benchmark samples")
			fs.DurationVarP(&opts.Time, "time", "t", 0, "Run each test for this duration per benchmark sample (e.g. 10s)")
			fs.BoolVar(&opts.Profile, "profile", false, "Report memory usage per run (allocs/op, B/op)")
			fs.StringVar(&opts.CPUProfile, "cpuprofile", "", "Write CPU profile to file")
			fs.StringVar(&opts.MemProfile, "memprofile", "", "Write memory profile to file")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, opts)
		},
	}
}

// fixtureRun aggregates the outcome of one fixture over one or more runs.
type fixtureRun struct {
	Result      *tests.TestResult
	DisplayPath string
	Runs        int
	Total       time.Duration
	P50         time.Duration
	P95         time.Duration
	P99         time.Duration
	AllocsPerOp uint64
	BytesPerOp  uint64
	GCRuns      uint32
}

func runFixtureLoop(ctx context.Context, fx *tests.Fixture, r tests.Runner, opts Options) *fixtureRun {
	out := &fixtureRun{}
	hint := opts.Count
	if hint <= 0 {
		hint = 64
	}
	samples := make([]int64, 0, hint)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
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
	runtime.ReadMemStats(&after)
	out.GCRuns = after.NumGC - before.NumGC
	if opts.Profile {
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

// Run executes .phpt test fixtures matching the provided paths or patterns.
func Run(ctx context.Context, args []string, opts Options) error {
	if opts.CPUProfile != "" {
		f, err := os.Create(opts.CPUProfile)
		if err != nil {
			return fmt.Errorf("create cpuprofile: %w", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return fmt.Errorf("start cpuprofile: %w", err)
		}
		defer func() {
			pprof.StopCPUProfile()
			f.Close()
		}()
	}

	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}

	fixtures, err := tests.FindFixtures(paths)
	if err != nil {
		return fmt.Errorf("discover fixtures: %w", err)
	}

	if len(fixtures) == 0 {
		if !opts.JSON {
			fmt.Println("No .phpt test fixtures found.")
		} else {
			_ = json.NewEncoder(os.Stdout).Encode(jsonReport{Results: []jsonFixture{}})
		}
		return nil
	}

	displayPaths := make([]string, len(fixtures))
	for i, fx := range fixtures {
		displayPaths[i] = fx.Path
		if displayPaths[i] == "" {
			displayPaths[i] = fx.Name
		}
	}

	if opts.Matrix {
		failed := runMatrix(ctx, fixtures, displayPaths, opts)
		if err := writeMemProfile(opts); err != nil {
			return err
		}
		if failed > 0 {
			return fmt.Errorf("%d fixture(s) failed", failed)
		}
		return nil
	}

	var table resultTable
	if !opts.JSON {
		table = newResultTable(os.Stdout, displayPaths, opts)
		table.writeHeader()
	}

	var jsonRows []jsonFixture
	var failedRuns []*fixtureRun
	var passedCount, failedCount int
	startAll := time.Now()

	for i, fx := range fixtures {
		var fixtureResult *tests.TestResult
		var firstFailedRun *fixtureRun
		for _, fr := range runFixtureSamples(ctx, fx, tests.RunnerRuntime, opts) {
			fr.DisplayPath = displayPaths[i]
			if table != nil {
				table.writeResult(fr)
			}
			jsonRows = append(jsonRows, jsonFixture{
				Name:        fx.Name,
				Path:        fr.DisplayPath,
				Runner:      string(fr.Result.Runner),
				Passed:      fr.Result.Passed,
				Runs:        fr.Runs,
				DurationNs:  fr.Total.Nanoseconds(),
				P50Ns:       fr.P50.Nanoseconds(),
				P95Ns:       fr.P95.Nanoseconds(),
				P99Ns:       fr.P99.Nanoseconds(),
				AllocsPerOp: fr.AllocsPerOp,
				BytesPerOp:  fr.BytesPerOp,
				GCRuns:      fr.GCRuns,
				Failure:     fr.Result.FailureReason,
			})

			if fixtureResult == nil || fixtureResult.Passed {
				fixtureResult = fr.Result
			}
			if !fr.Result.Passed && firstFailedRun == nil {
				firstFailedRun = fr
			}
		}

		if fixtureResult.Passed {
			passedCount++
		} else {
			failedCount++
			failedRuns = append(failedRuns, firstFailedRun)
		}
	}

	if table != nil {
		table.close()
		table.writeSummary(passedCount, failedCount, len(fixtures), time.Since(startAll))
	}

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(jsonReport{
			Total:   len(fixtures),
			Passed:  passedCount,
			Failed:  failedCount,
			Results: jsonRows,
		}); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	}

	if !opts.JSON {
		for _, fr := range failedRuns {
			fmt.Printf("FAIL %s (%dms)\n", fr.DisplayPath, fr.Result.DurationMs)
			if fr.Result.FailureReason != "" {
				fmt.Printf("  Reason: %s\n", fr.Result.FailureReason)
			}
		}
	}

	if err := writeMemProfile(opts); err != nil {
		return err
	}

	if failedCount > 0 {
		return fmt.Errorf("%d fixture(s) failed", failedCount)
	}

	return nil
}

// writeMemProfile dumps a heap profile when one was requested.
func writeMemProfile(opts Options) error {
	if opts.MemProfile == "" {
		return nil
	}
	f, err := os.Create(opts.MemProfile)
	if err != nil {
		return fmt.Errorf("create memprofile: %w", err)
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("write memprofile: %w", err)
	}
	return nil
}
