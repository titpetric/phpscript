package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/internal/table"
	"github.com/titpetric/phpscript/tests"
)

// Name is the command title.
const Name = "Run .phpt test fixtures"

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

// NewCommand creates a new test command.
func NewCommand(cfg *config.Config, globals *flags.Options) *cli.Command {
	appConfig := config.Value(cfg)

	var opts Options
	var bound *cli.FlagSet
	return &cli.Command{
		Name:  "test",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			// Kept so Run can read which flags the command line carries. A
			// configuration supplies a default and a flag overrides it, and
			// the value alone cannot tell the two apart: an operator forcing
			// -p 1 over a file naming 4 means it.
			bound = fs
			fs.BoolVar(&opts.JSON, "json", false, "Write machine-readable JSON to stdout")
			fs.BoolVar(&opts.Matrix, "matrix", false, "Run every fixture through all runtimes and report a matrix")
			fs.BoolVar(&opts.SkipPHP, "skip-php", false, "With --matrix, leave the php binary out: the built-in runtimes alone")
			fs.IntVarP(&opts.Parallel, "parallel", "p", 1, "Run up to N fixtures concurrently")
			fs.IntVarP(&opts.Count, "count", "c", 0, "Run each test N times; with --time, produce N benchmark samples")
			fs.DurationVarP(&opts.Time, "time", "t", 0, "Run each test for this duration per benchmark sample (e.g. 10s)")
			fs.BoolVar(&opts.Profile, "profile", false, "Report memory usage per run (allocs/op, B/op)")
			fs.StringVarP(&opts.Output, "output", "o", "", "Write a Markdown report of the results to this file")
			fs.BoolVar(&opts.Split, "split", false, "With --cover, also write each fixture's own coverage next to it as <fixture>.cov")
			fs.StringVar(&opts.Cache, "cache", CacheWorker, "Scope of the parse caches: worker (one per worker loop, the default) or off (a new one per fixture run)")
		},
		Run: func(ctx context.Context, args []string) error {
			opts.Include = globals.Include
			opts.Verbose = globals.Verbose
			opts.Cover = globals.Cover
			opts.CoverFile = globals.CoverFile
			opts.Config = appConfig
			opts.typed = func(name string) bool { return bound != nil && bound.Changed(name) }
			return Run(ctx, args, opts)
		},
	}
}

// Run executes .phpt test fixtures matching the paths or patterns given.
func Run(ctx context.Context, args []string, opts Options) error {
	paths := args
	if len(paths) == 0 {
		// A bare invocation means the whole tree: a pipeline that names no
		// paths runs from the application root, where the fixtures live in
		// the folders below. An explicit "." keeps its non-recursive meaning.
		paths = []string{"./..."}
	}

	// Suite discovery comes before the flag checks in run, because the
	// run-wide options a phpscript.yml carries are among what run checks.
	found, err := discoverSuites(opts.Config, paths)
	if err != nil {
		return err
	}
	if found.run != nil {
		opts.fromConfig(found.run.Config.Test)
	} else {
		opts.fromConfig(opts.Config.Test)
	}

	return run(ctx, args, paths, found, opts)
}

func run(ctx context.Context, args, paths []string, found suites, opts Options) error {
	if opts.Parallel < 0 {
		return fmt.Errorf("parallel must be at least 1")
	}
	if opts.Parallel == 0 {
		opts.Parallel = 1
	}
	if opts.Parallel > 1 && opts.Profile {
		return fmt.Errorf("profile cannot be combined with parallel fixture execution")
	}
	if opts.Cover == "" && opts.Split {
		opts.Cover = CoverLine
	}
	if opts.Cover != "" {
		// A coverage count is how many times the fixture reached a line and a
		// benchmark loop would multiply every count by its repetitions.
		if opts.Count > 0 || opts.Time > 0 {
			return fmt.Errorf("cover cannot be combined with --count or --time")
		}
		// A report mode and --json both own stdout.
		if opts.coverReport() && opts.JSON {
			return fmt.Errorf("cover=%s cannot be combined with --json", opts.Cover)
		}
		if opts.CoverFile == "" {
			opts.CoverFile = DefaultCoverFile
		}
	}

	fixtures, err := tests.FindFixtures(paths)
	if err != nil {
		return fmt.Errorf("discover fixtures: %w", err)
	}

	mode, err := cacheMode(opts.Cache)
	if err != nil {
		return err
	}
	for _, fx := range fixtures {
		fx.SetCacheScope(mode)
	}

	applySuites(fixtures, found, opts)

	// An empty selection is a mis-scoped invocation, not a passing run: a
	// pipeline that points the runner at the wrong directory should fail
	// and never reports success over nothing.
	if len(fixtures) == 0 {
		// The empty report still goes out, the way a failing run's does at the
		// end of this function: a consumer reading stdout gets well-formed JSON
		// either way, and the error is what carries the exit code.
		if opts.JSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonReport{Results: []jsonFixture{}})
		}
		return fmt.Errorf("no .phpt test fixtures found in %s", strings.Join(paths, " "))
	}

	if opts.Cover != "" {
		coverFixtures(fixtures)
	}

	displayPaths := make([]string, len(fixtures))
	for i, fx := range fixtures {
		displayPaths[i] = fx.Path
		if displayPaths[i] == "" {
			displayPaths[i] = fx.Name
		}
	}
	groups := groupFixtures(fixtures, displayPaths)

	report, err := openReport(opts, args)
	if err != nil {
		return err
	}
	if report != nil {
		defer report.Close()
	}

	// The state a suite lays down is what its fixtures assert against and a
	// setup that failed ends the run before a fixture reports the same missing
	// table once per file. Teardown runs whatever the fixtures did, and its
	// failure prints on stderr without displacing theirs: a suite that could
	// not clean up says less than a fixture that did not pass.
	hooks := found.all()
	if err := tests.RunSetup(ctx, hooks, hookOutput(opts)); err != nil {
		return err
	}
	defer func() {
		if err := tests.RunTeardown(ctx, hooks, hookOutput(opts)); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	if opts.Matrix {
		failed := runMatrix(ctx, groups, opts, report)
		// Only the runtime column collects: flatstack carries no coverage
		// support and the php column is another process.
		if opts.Cover != "" {
			if err := writeCoverage(opts, fixtures); err != nil {
				return err
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d fixture(s) failed", failed)
		}
		return nil
	}

	var sinks teeResultTable
	if !opts.JSON && !opts.coverReport() && opts.Verbose {
		// A report mode leaves stdout to the coverage report so it pipes
		// clean; -o still writes the tables as Markdown. Without -v the
		// fixture tables stay off stdout entirely and the folder summary
		// below is the whole answer: a tree of a few hundred fixtures prints
		// a few hundred rows, and reading them is not what a run is for.
		sinks = append(sinks, newResultTable(os.Stdout, opts, !table.IsTerminal(os.Stdout)))
	}
	if report != nil {
		sinks = append(sinks, newMarkdownTable(report, opts))
	}

	var jsonRows []jsonFixture
	var failedRuns []*fixtureRun
	var passedCount, failedCount int
	var folders []folderSummary
	startAll := time.Now()

	for _, group := range groups {
		sinks.writeGroup(group.Dir, group.Labels)
		groupPassed, groupFailed := 0, 0
		groupStart := time.Now()

		fixtureRuns := mapFixtures(group.Fixtures, opts.Parallel, func(worker, _ int, fx *tests.Fixture) []*fixtureRun {
			return runFixtureSamples(tests.WithWorker(ctx, worker), fx, tests.RunnerRuntime, opts)
		})
		for i, fx := range group.Fixtures {
			var fixtureResult *tests.TestResult
			var firstFailedRun *fixtureRun
			for _, fr := range fixtureRuns[i] {
				fr.DisplayPath = group.Paths[i]
				fr.Label = group.Labels[i]
				fr.Fixture = fx
				sinks.writeResult(fr)
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
				groupPassed++
			} else {
				groupFailed++
				failedRuns = append(failedRuns, firstFailedRun)
			}
		}

		totals := groupTotals{
			Dir:      group.Dir,
			Passed:   groupPassed,
			Failed:   groupFailed,
			Total:    len(group.Fixtures),
			Duration: time.Since(groupStart),
		}
		sinks.closeGroup(totals)
		folders = append(folders, folderSummary{groupTotals: totals})
		passedCount += groupPassed
		failedCount += groupFailed
	}

	// A coverage reading needs every fixture to have run and the per-folder
	// measure happens here and not as each folder closed.
	var covers []folderCover
	if opts.Cover != "" && !opts.JSON && !opts.coverReport() {
		covers = folderCoverage(groups)
	}

	// The folder summary is what a run answers with when it prints no fixture
	// table: one row per folder resolved from the arguments, in place of one
	// table per fixture.
	if !opts.JSON && !opts.coverReport() && !opts.Verbose {
		for i := range covers {
			folders[i].cover = &covers[i]
		}
		markdown := !table.IsTerminal(os.Stdout)
		writeFolderTable(os.Stdout, folders, markdown, opts.Time > 0 || opts.Count > 0 || opts.Profile)
		fmt.Printf("Test summary: %d passed, %d failed out of %d fixtures (%dms)\n",
			passedCount, failedCount, len(fixtures), time.Since(startAll).Milliseconds())
	}

	sinks.writeSummary(passedCount, failedCount, len(fixtures), time.Since(startAll))

	// With -v, coverage drops to the file level, under the folder loading it.
	// A fixture table cannot report that: its column says how much of what a
	// fixture loads runs, and never which file goes unvisited.
	// A folder whose fixtures loaded no PHP file of their own has no section.
	if opts.Verbose {
		writeFolderFileReport(os.Stdout, covers)
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
		// Failures stay visible in a report mode, on stderr, where they do not
		// corrupt the piped report.
		failOut := os.Stdout
		if opts.coverReport() {
			failOut = os.Stderr
		}
		for _, fr := range failedRuns {
			fmt.Fprintf(failOut, "FAIL %s (%dms)\n", fr.DisplayPath, fr.Result.DurationMs)
			if fr.Result.FailureReason != "" {
				fmt.Fprintf(failOut, "  Reason: %s\n", fr.Result.FailureReason)
			}
		}
	}

	if opts.Cover != "" {
		if err := writeCoverage(opts, fixtures); err != nil {
			return err
		}
	}

	if failedCount > 0 {
		return fmt.Errorf("%d fixture(s) failed", failedCount)
	}

	return nil
}
