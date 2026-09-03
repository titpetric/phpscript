package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/metrics"
	"sort"
	"strings"
	"time"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/internal/table"
	"github.com/titpetric/phpscript/tests"
)

// Name is the command title.
const Name = "Run .phpt test fixtures"

// Options holds CLI flag options for the test command. Include, Verbose, Cover
// and CoverFile are the shared flags, copied off internal/flags before the run
// so the report writers keep taking one struct.
type Options struct {
	Include   string
	JSON      bool
	Matrix    bool
	Verbose   bool
	Parallel  int
	Count     int
	Time      time.Duration
	Profile   bool
	Output    string
	Cover     string
	CoverFile string
	Split     bool
	SkipPHP   bool
	Cache     string
}

// coverReport reports whether the cover mode owns stdout with a per-symbol
// report, which is what suppresses the fixture tables.
func (o Options) coverReport() bool {
	return o.Cover == CoverFunc || o.Cover == CoverFile
}

// runners answers the matrix columns this invocation covers: every backend,
// minus php when --skip-php dropped the external binary from the run. The
// column leaves the table entirely rather than reporting SKIP per row - a
// skipped column says the machine has no php, this flag says do not ask.
func (o Options) runners() []tests.Runner {
	if !o.SkipPHP {
		return tests.Runners
	}
	runners := make([]tests.Runner, 0, len(tests.Runners))
	for _, r := range tests.Runners {
		if r != tests.RunnerPHP {
			runners = append(runners, r)
		}
	}
	return runners
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

// NewCommand creates a new test command.
func NewCommand(globals *flags.Options) *cli.Command {
	var opts Options
	return &cli.Command{
		Name:  "test",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
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
			return Run(ctx, args, opts)
		},
	}
}

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
// DisplayPath is the full path, which is what a failure line and the JSON
// report name; Label is the basename, which is what a per-folder table shows.
type fixtureRun struct {
	Result      *tests.TestResult
	DisplayPath string
	Label       string
	// Fixture is the fixture the run came from, carried so a table can read
	// the coverage collector installed on it. It is nil for a run assembled
	// by a test.
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
	// matrix - for two numbers nothing prints unless --profile asked for them.
	// The GC count comes from runtime/metrics either way, which does not stop
	// anything, so the column stays.
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
		// inside the one pair of readings, so the totals are divided by the
		// runs rather than sampled per iteration.
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
		// A coverage count is how many times the fixture reached a line, so a
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

	paths := args
	if len(paths) == 0 {
		// A bare invocation means the whole tree: a pipeline that names no
		// paths runs from the application root, where the fixtures live in
		// the folders below. An explicit "." keeps its non-recursive meaning.
		paths = []string{"./..."}
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

	if opts.Include != "" {
		// The flag speaks from the invocation root: that is where the include
		// file lives, below no fixture directory. It is the one mechanism -
		// composer's autoloader resolves the classes, and the file's own
		// includes bring the helpers, so a second folder convention on the
		// command line had nothing left to do.
		for _, fx := range fixtures {
			fx.SetAppRoot(".", opts.Include)
		}
	}

	// An empty selection is a mis-scoped invocation, not a passing run: a
	// pipeline that points the runner at the wrong directory should fail
	// rather than report success over nothing.
	if len(fixtures) == 0 {
		if opts.JSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonReport{Results: []jsonFixture{}})
			return nil
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

	if opts.Matrix {
		failed := runMatrix(ctx, groups, opts, report)
		// Only the runtime column collects: flatstack carries no coverage
		// support and the php column is another process.
		if opts.Cover != "" {
			if err := writeCoverage(fixtures, opts); err != nil {
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
	var passedCount, failedCount, skippedCount int
	var folders []folderSummary
	startAll := time.Now()

	for _, group := range groups {
		sinks.writeGroup(group.Dir, group.Labels)
		groupPassed, groupFailed, groupSkipped := 0, 0, 0
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
					Skipped:     fr.Result.Skipped,
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
				if !fr.Result.Passed && !fr.Result.Skipped && firstFailedRun == nil {
					firstFailedRun = fr
				}
			}

			// A fixture the host cannot run, a Go plugin in a build without
			// cgo or a missing php binary, is neither a pass nor a failure: it
			// is a statement about the host. Counting it as a failure would
			// make a static build report a broken tree.
			switch {
			case fixtureResult.Skipped:
				groupSkipped++
			case fixtureResult.Passed:
				groupPassed++
			default:
				groupFailed++
				failedRuns = append(failedRuns, firstFailedRun)
			}
		}

		totals := groupTotals{
			Dir:      group.Dir,
			Passed:   groupPassed,
			Failed:   groupFailed,
			Total:    len(group.Fixtures) - groupSkipped,
			Duration: time.Since(groupStart),
		}
		sinks.closeGroup(totals)
		folders = append(folders, folderSummary{groupTotals: totals})
		passedCount += groupPassed
		failedCount += groupFailed
		skippedCount += groupSkipped
	}

	// Coverage is measured once every fixture has run, so the per-folder
	// reading is assembled here rather than as each folder closed.
	var covers []folderCover
	if opts.Cover != "" && !opts.JSON && !opts.coverReport() {
		covers = folderCoverage(groups)
	}

	// The folder summary is what a run answers with when the fixture tables are
	// not being printed: one row per folder resolved from the arguments,
	// instead of one table per fixture.
	if !opts.JSON && !opts.coverReport() && !opts.Verbose {
		for i := range covers {
			folders[i].cover = &covers[i]
		}
		markdown := !table.IsTerminal(os.Stdout)
		writeFolderTable(os.Stdout, folders, markdown, opts.Time > 0 || opts.Count > 0 || opts.Profile)
		fmt.Printf("Test summary: %d passed, %d failed out of %d fixtures (%dms)\n",
			passedCount, failedCount, len(fixtures)-skippedCount, time.Since(startAll).Milliseconds())
	}

	sinks.writeSummary(passedCount, failedCount, len(fixtures)-skippedCount, time.Since(startAll))

	// With -v, coverage drops to the file it was measured on, under the folder
	// that loaded it. That is the reading a fixture table cannot give: a column
	// says how much of what a fixture loaded ran, not which file went unvisited.
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
		if err := writeCoverage(fixtures, opts); err != nil {
			return err
		}
	}

	if failedCount > 0 {
		return fmt.Errorf("%d fixture(s) failed", failedCount)
	}

	return nil
}

// gcCycles returns the number of completed GC cycles.
//
// runtime/metrics rather than runtime.MemStats: NumGC is only reachable through
// ReadMemStats, which stops the world, and this is read twice per fixture per
// runner. The metrics reader takes a lock and copies a counter.
func gcCycles() uint64 {
	sample := [1]metrics.Sample{{Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(sample[:])
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}

// The --cache modes, which say how far a parsed include and a compiled
// expression travel.
//
// There is no "shared" mode because a worker loop already is one: run without
// --parallel there is a single worker, so a single set of caches serves the
// whole run. Sharing them across workers on top of that only adds contention
// for a hit the worker's own cache already has.
const (
	// CacheOff gives every fixture run its own caches and drops the runtime
	// after it. Nothing one run parsed is visible to the next, so a run is
	// charged the parsing its own includes cost - and what it held is returned
	// when it ends rather than kept for the length of the suite.
	CacheOff = "off"
	// CacheWorker gives each worker one set, reused by the fixtures that
	// worker runs serially. What is held scales with --parallel rather than
	// with the number of fixtures.
	CacheWorker = "worker"
)

// cacheMode reads the --cache flag.
func cacheMode(value string) (string, error) {
	switch value {
	case "", CacheWorker:
		return CacheWorker, nil
	case CacheOff:
		return value, nil
	}
	return "", fmt.Errorf("--cache must be %s or %s, not %q", CacheWorker, CacheOff, value)
}
