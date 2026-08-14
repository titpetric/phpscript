package lint

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/titpetric/cli"

	phplint "github.com/titpetric/phpscript/lint"
	"github.com/titpetric/phpscript/list"
)

// Name is the command title.
const Name = "Lint php scripts"

// NewCommand creates a new lint command.
func NewCommand() *cli.Command {
	var checkFlatstack bool

	return &cli.Command{
		Name:  "lint",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&checkFlatstack, "flatstack", false, "Check flatstack bytecode engine compatibility and print diagnostic reason if unsupported")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(args, checkFlatstack)
		},
	}
}

// Run lints files or directories and reports diagnostics.
func Run(args []string, checkFlatstack bool) error {
	return run(args, checkFlatstack, os.Stdout)
}

type result struct {
	status  string
	file    string
	line    int
	message string
}

func run(args []string, checkFlatstack bool, out io.Writer) error {
	files, err := list.ExpandFiles(args)
	if err != nil {
		return err
	}

	var results []result
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		diags, err := phplint.File(file, string(src))
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		for _, d := range diags {
			status := "WARN"
			if strings.HasPrefix(d.Message, "parse error:") || strings.HasPrefix(d.Message, "fixture parse error:") {
				status = "FAIL"
			}
			results = append(results, result{status: status, file: d.File, line: d.Line, message: d.Message})
		}

		if checkFlatstack && !hasFailure(diags) {
			d, err := phplint.FlatstackFile(file, string(src))
			if err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			if strings.HasPrefix(d.Message, "[flatstack unsupported]") {
				results = append(results, result{status: "FAIL", file: d.File, line: d.Line, message: d.Message})
			} else if len(diags) == 0 {
				results = append(results, result{status: "PASS", file: d.File, line: d.Line, message: d.Message})
			}
		} else if len(diags) == 0 {
			results = append(results, result{status: "PASS", file: file, message: "No lint findings"})
		}
	}

	passing, warnings, failing := writeResults(out, results)
	fmt.Fprintf(out, "\nPassing %d, with %d warnings, %d failing\n", passing, warnings, failing)
	if failing > 0 {
		return fmt.Errorf("lint completed with %d failing checks", failing)
	}
	return nil
}

func hasFailure(diags []phplint.Diagnostic) bool {
	for _, d := range diags {
		if strings.HasPrefix(d.Message, "parse error:") || strings.HasPrefix(d.Message, "fixture parse error:") {
			return true
		}
	}
	return false
}

func writeResults(w io.Writer, results []result) (passing, warnings, failing int) {
	fmt.Fprintln(w, "| Status | File | Line | Message |")
	fmt.Fprintln(w, "| --- | --- | ---: | --- |")
	for _, r := range results {
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", r.status, markdownCell(r.file), line(r.line), markdownCell(r.message))
		switch r.status {
		case "PASS":
			passing++
		case "WARN":
			warnings++
		case "FAIL":
			failing++
		}
	}
	return passing, warnings, failing
}

func line(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func markdownCell(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	return strings.ReplaceAll(value, "|", `\|`)
}
