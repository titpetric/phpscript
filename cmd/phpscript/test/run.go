package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, opts)
		},
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
	var passedCount, failedCount int
	startAll := time.Now()

	for _, fx := range fixtures {
		res := tests.RunFixture(ctx, fx)
		results = append(results, res)

		displayPath := fx.Path
		if displayPath == "" {
			displayPath = fx.Name
		}

		if res.Passed {
			passedCount++
			fmt.Printf("PASS %s (%dms)\n", displayPath, res.DurationMs)
		} else {
			failedCount++
			fmt.Printf("FAIL %s (%dms)\n", displayPath, res.DurationMs)
			if res.FailureReason != "" {
				fmt.Printf("  Reason: %s\n", res.FailureReason)
			}
		}
	}

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
		if err := os.MkdirAll(dir, 0755); err != nil {
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
