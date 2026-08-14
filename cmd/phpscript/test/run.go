package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/tests"
)

// Name is the command title.
const Name = "Run .phpt test fixtures"

// Options holds CLI flag options for the test command.
type Options struct {
	Report     string
	ReportHTML string
	Count      int
	Time       time.Duration
	Profile    bool
}

// NewCommand creates a new test command.
func NewCommand() *cli.Command {
	var opts Options
	return &cli.Command{
		Name:  "test",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.StringVar(&opts.Report, "report", "", "Write JSON test report to specified file")
			fs.StringVar(&opts.ReportHTML, "report-html", "", "Write HTML test report to specified file")
			fs.IntVar(&opts.Count, "count", 0, "Run each test N times; with --time, produce N benchmark samples")
			fs.DurationVar(&opts.Time, "time", 0, "Run each test for this duration per benchmark sample (e.g. 10s)")
			fs.BoolVar(&opts.Profile, "profile", false, "Report memory usage per run (allocs/op, B/op)")
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
	AllocsPerOp uint64
	BytesPerOp  uint64
	GCRuns      uint32
}

// runFixtureLoop reruns one fixture until the count and time requirements are
// both met (a single run when neither flag is set). The reported result is the
// first failing run if any, else the last run.
func runFixtureLoop(ctx context.Context, fx *tests.Fixture, opts Options) *fixtureRun {
	out := &fixtureRun{}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	for {
		res := tests.RunFixture(ctx, fx)
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
	return out
}

// runFixtureSamples returns one aggregate run normally. When count and time
// are both set, count selects the number of benchmark samples and each sample
// runs independently for the requested time.
func runFixtureSamples(ctx context.Context, fx *tests.Fixture, opts Options) []*fixtureRun {
	if opts.Count <= 0 || opts.Time <= 0 {
		return []*fixtureRun{runFixtureLoop(ctx, fx, opts)}
	}

	sampleOpts := opts
	sampleOpts.Count = 0
	runs := make([]*fixtureRun, 0, opts.Count)
	for range opts.Count {
		runs = append(runs, runFixtureLoop(ctx, fx, sampleOpts))
		if ctx.Err() != nil {
			break
		}
	}
	return runs
}

// Run executes .phpt test fixtures matching the provided paths or patterns.
func Run(ctx context.Context, args []string, opts Options) error {
	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}

	fixtures, err := tests.FindFixtures(paths)
	if err != nil {
		return fmt.Errorf("discover fixtures: %w", err)
	}

	if len(fixtures) == 0 {
		fmt.Println("No .phpt test fixtures found.")
		return nil
	}

	displayPaths := make([]string, len(fixtures))
	for i, fx := range fixtures {
		displayPaths[i] = fx.Path
		if displayPaths[i] == "" {
			displayPaths[i] = fx.Name
		}
	}
	table := newResultTable(os.Stdout, displayPaths, opts)
	table.writeHeader()

	var results []*tests.TestResult
	var failedRuns []*fixtureRun
	var passedCount, failedCount int
	startAll := time.Now()

	for i, fx := range fixtures {
		var fixtureResult *tests.TestResult
		var firstFailedRun *fixtureRun
		for _, fr := range runFixtureSamples(ctx, fx, opts) {
			fr.DisplayPath = displayPaths[i]
			table.writeResult(fr)

			if fixtureResult == nil || fixtureResult.Passed {
				fixtureResult = fr.Result
			}
			if !fr.Result.Passed && firstFailedRun == nil {
				firstFailedRun = fr
			}
		}
		results = append(results, fixtureResult)

		if fixtureResult.Passed {
			passedCount++
		} else {
			failedCount++
			failedRuns = append(failedRuns, firstFailedRun)
		}
	}
	table.close()
	table.writeSummary(passedCount, failedCount, len(fixtures), time.Since(startAll))

	for _, fr := range failedRuns {
		fmt.Printf("FAIL %s (%dms)\n", fr.DisplayPath, fr.Result.DurationMs)
		if fr.Result.FailureReason != "" {
			fmt.Printf("  Reason: %s\n", fr.Result.FailureReason)
		}
	}

	if opts.Report != "" {
		if err := writeReportFile(opts.Report, func(f *os.File) error {
			return tests.WriteJSONReport(f, results)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON report: %v\n", err)
		} else {
			fmt.Printf("Wrote JSON report to %s\n", opts.Report)
		}
	}

	if opts.ReportHTML != "" {
		if err := writeReportFile(opts.ReportHTML, func(f *os.File) error {
			return tests.WriteHTMLReport(f, results)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing HTML report: %v\n", err)
		} else {
			fmt.Printf("Wrote HTML report to %s\n", opts.ReportHTML)
		}
	}

	if failedCount > 0 {
		return fmt.Errorf("%d fixture(s) failed", failedCount)
	}

	return nil
}

func writeReportFile(path string, writeFn func(*os.File) error) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeFn(f)
}
