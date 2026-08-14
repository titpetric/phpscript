package test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
			fs.IntVar(&opts.Count, "count", 0, "Run each test the set amount of times")
			fs.DurationVar(&opts.Time, "time", 0, "Rerun each test in a loop for the given duration (e.g. 10s)")
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
}

// runFixtureLoop reruns one fixture until the count and time requirements are
// both met (a single run when neither flag is set). The reported result is the
// first failing run if any, else the last run.
func runFixtureLoop(ctx context.Context, fx *tests.Fixture, opts Options) *fixtureRun {
	out := &fixtureRun{}
	var before, after runtime.MemStats
	if opts.Profile {
		runtime.ReadMemStats(&before)
	}
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
	if opts.Profile {
		runtime.ReadMemStats(&after)
		n := uint64(out.Runs)
		out.AllocsPerOp = (after.Mallocs - before.Mallocs) / n
		out.BytesPerOp = (after.TotalAlloc - before.TotalAlloc) / n
	}
	return out
}

// writeMarkdownTable prints the benchmark-style summary table. Base columns
// are Status, Filename, Duration; Count is added when -count/-time is in
// effect, Allocations/op and Bytes/op when -profile is.
func writeMarkdownTable(w io.Writer, runs []*fixtureRun, opts Options) {
	loop := opts.Count > 0 || opts.Time > 0
	header := []string{"Status", "Filename", "Duration"}
	if loop {
		header = append(header, "Count")
	}
	if opts.Profile {
		header = append(header, "Allocations/op", "Bytes/op")
	}
	fmt.Fprintln(w, "| "+strings.Join(header, " | ")+" |")
	sep := make([]string, len(header))
	for i := range sep {
		sep[i] = "--"
	}
	fmt.Fprintln(w, "| "+strings.Join(sep, " | ")+" |")
	for _, r := range runs {
		status := "PASS"
		if !r.Result.Passed {
			status = "FAIL"
		}
		row := []string{status, r.DisplayPath, r.Total.Round(time.Microsecond).String()}
		if loop {
			row = append(row, strconv.Itoa(r.Runs))
		}
		if opts.Profile {
			row = append(row, strconv.FormatUint(r.AllocsPerOp, 10), strconv.FormatUint(r.BytesPerOp, 10))
		}
		fmt.Fprintln(w, "| "+strings.Join(row, " | ")+" |")
	}
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

	var results []*tests.TestResult
	var runs []*fixtureRun
	var passedCount, failedCount int
	startAll := time.Now()

	for _, fx := range fixtures {
		fr := runFixtureLoop(ctx, fx, opts)
		res := fr.Result
		results = append(results, res)

		fr.DisplayPath = fx.Path
		if fr.DisplayPath == "" {
			fr.DisplayPath = fx.Name
		}
		runs = append(runs, fr)

		if res.Passed {
			passedCount++
		} else {
			failedCount++
			fmt.Printf("FAIL %s (%dms)\n", fr.DisplayPath, res.DurationMs)
			if res.FailureReason != "" {
				fmt.Printf("  Reason: %s\n", res.FailureReason)
			}
		}
	}

	writeMarkdownTable(os.Stdout, runs, opts)

	totalDur := time.Since(startAll).Milliseconds()
	fmt.Printf("\nTest summary: %d passed, %d failed out of %d fixtures (%dms)\n", passedCount, failedCount, len(fixtures), totalDur)

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
