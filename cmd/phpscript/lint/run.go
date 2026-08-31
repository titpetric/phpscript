package lint

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/internal/table"
	phplint "github.com/titpetric/phpscript/lint"
	"github.com/titpetric/phpscript/list"
)

// Name is the command title.
const Name = "Lint php scripts"

// Options holds CLI flag options for the lint command.
type Options struct {
	Flatstack bool
	Output    string
}

// NewCommand creates a new lint command.
func NewCommand() *cli.Command {
	var opts Options

	return &cli.Command{
		Name:  "lint",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&opts.Flatstack, "flatstack", false, "Check flatstack bytecode engine compatibility and print diagnostic reason if unsupported")
			fs.StringVarP(&opts.Output, "output", "o", "", "Write a Markdown report of the findings to this file")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(args, opts)
		},
	}
}

// Run lints files or directories and reports diagnostics.
func Run(args []string, opts Options) error {
	return run(args, opts, os.Stdout)
}

type result struct {
	status  string
	file    string
	line    int
	message string
}

const (
	statusPass = "PASS"
	statusWarn = "WARN"
	statusFail = "FAIL"
)

func run(args []string, opts Options, out io.Writer) error {
	results, err := collect(args, opts.Flatstack)
	if err != nil {
		return err
	}

	// The report file is created before anything is printed, so an unwritable
	// path fails the command rather than leaving half a report behind.
	var report *os.File
	if opts.Output != "" {
		report, err = os.Create(opts.Output)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer report.Close()
		writeReportHeader(report, reportArgs(opts, args))
	}

	// Stdout keeps the ansi tables on a terminal and falls back to markdown
	// when it is redirected; the report file is always markdown.
	sinks := []sink{{w: out, markdown: !table.IsTerminal(out)}}
	if report != nil {
		sinks = append(sinks, sink{w: report, markdown: true})
	}

	// One table per folder scanned, the way `phpscript test` reports: the
	// folder is the header of the file column, the rows show basenames, and a
	// summary line closes each folder before the run's total.
	files := make([]string, len(results))
	for i, r := range results {
		files[i] = r.file
	}

	var passing, warnings, failing int
	for _, group := range list.GroupByDir(files) {
		tables := make([]*table.Table, len(sinks))
		for i, s := range sinks {
			if s.markdown {
				fmt.Fprintf(s.w, "## %s\n\n", group.Dir)
			}
			tables[i] = newTable(s.w, s.markdown, group.Dir)
		}

		groupPassing, groupWarnings, groupFailing := 0, 0, 0
		for _, at := range group.Indexes {
			r := results[at]
			switch r.status {
			case statusPass:
				groupPassing++
			case statusWarn:
				groupWarnings++
			case statusFail:
				groupFailing++
			}
			for _, sink := range tables {
				sink.Row(
					table.Colored(statusColor(r.status), r.status),
					table.Colored(table.ColorAmber, filepath.Base(r.file)),
					table.Colored(table.ColorWhite, line(r.line)),
					table.Colored(table.ColorWhite, r.message),
				)
			}
		}

		for _, sink := range tables {
			sink.Flush()
			sink.Summary("%s: %d passing, %d warnings, %d failing", group.Dir, groupPassing, groupWarnings, groupFailing)
		}
		passing += groupPassing
		warnings += groupWarnings
		failing += groupFailing
	}

	for _, s := range sinks {
		s.summary("Passing %d, with %d warnings, %d failing", passing, warnings, failing)
	}

	if failing > 0 {
		return fmt.Errorf("lint completed with %d failing checks", failing)
	}
	return nil
}

// sink is one output the findings go to: the terminal or a report file, each
// holding a table per folder rather than one table for the run.
type sink struct {
	w        io.Writer
	markdown bool
}

// summary writes the run's closing line the way a table's summary prints, so
// the total reads like the per-folder lines above it.
func (s sink) summary(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if !s.markdown {
		text = table.ColorHeader + text + table.ColorReset
	}
	fmt.Fprintf(s.w, "\n%s\n", text)
}

// collect lints every file and returns one result per finding, plus a passing
// result for a file that had none, so the table reports what was checked
// rather than only what went wrong.
func collect(args []string, checkFlatstack bool) ([]result, error) {
	files, err := list.ExpandFiles(args)
	if err != nil {
		return nil, err
	}

	var results []result
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}

		diags, err := phplint.File(file, string(src))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for _, d := range diags {
			status := statusWarn
			if d.Fatal || isParseError(d.Message) {
				status = statusFail
			}
			results = append(results, result{status: status, file: d.File, line: d.Line, message: d.Message})
		}

		if checkFlatstack && !hasFailure(diags) {
			d, err := phplint.FlatstackFile(file, string(src))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", file, err)
			}
			if strings.HasPrefix(d.Message, "[flatstack unsupported]") {
				results = append(results, result{status: statusFail, file: d.File, line: d.Line, message: d.Message})
			} else if len(diags) == 0 {
				results = append(results, result{status: statusPass, file: d.File, line: d.Line, message: d.Message})
			}
		} else if len(diags) == 0 {
			results = append(results, result{status: statusPass, file: file, message: "No lint findings"})
		}
	}
	return results, nil
}

// newTable returns one folder's findings table. The folder names the file
// column, so a table says which area it covers without spending a line on a
// title. The line number is right aligned because it is a number; everything
// else reads as a label.
func newTable(w io.Writer, markdown bool, dir string) *table.Table {
	if dir == "" {
		dir = "File"
	}
	return table.New(w, markdown,
		table.Column{Title: "Status"},
		table.Column{Title: dir},
		table.Column{Title: "Line", Align: table.Right},
		table.Column{Title: "Message"},
	)
}

func statusColor(status string) string {
	switch status {
	case statusFail:
		return table.ColorRed
	case statusWarn:
		return table.ColorAmber
	default:
		return table.ColorGreen
	}
}

func hasFailure(diags []phplint.Diagnostic) bool {
	for _, d := range diags {
		if isParseError(d.Message) {
			return true
		}
	}
	return false
}

func isParseError(message string) bool {
	return strings.HasPrefix(message, "parse error:") || strings.HasPrefix(message, "fixture parse error:")
}

func line(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// writeReportHeader opens a generated markdown report with its title and the
// command that produced it, so a checked-in report says how to regenerate it.
func writeReportHeader(w io.Writer, argv []string) {
	command := strings.Join(append([]string{"phpscript"}, argv...), " ")

	fmt.Fprint(w, "# Lint findings\n\n")
	fmt.Fprintf(w, "<!-- Generated by: %s -->\n\n", command)
	fmt.Fprint(w, "One table per folder scanned, one row per finding, and one row per file that\n")
	fmt.Fprint(w, "had none. A parse error fails the run; every other finding is a warning.\n\n")
}

// reportArgs reconstructs the lint invocation for the provenance comment.
func reportArgs(opts Options, args []string) []string {
	argv := []string{"lint"}
	if opts.Flatstack {
		argv = append(argv, "--flatstack")
	}
	if opts.Output != "" {
		argv = append(argv, "-o", filepath.ToSlash(opts.Output))
	}
	return append(argv, args...)
}
