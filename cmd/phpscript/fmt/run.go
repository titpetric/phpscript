package fmt

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/formatter"
)

// Name is the command title.
const Name = "Format php scripts"

// NewCommand creates a new fmt command.
func NewCommand() *cli.Command {
	var list bool

	return &cli.Command{
		Name:  "fmt",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVarP(&list, "list", "l", false, "List the files that need formatting instead of rewriting them")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(args, list)
		},
	}
}

// Run formats files or directories in place by pretty-printing the AST. Files
// the formatter cannot read in full are reported and left alone, so one file
// using PHP that phpscript does not support does not stop the rest.
func Run(args []string, list bool) error {
	return run(args, list, os.Stdout, os.Stderr)
}

func run(args []string, list bool, out, errOut io.Writer) error {
	// Listing runs the same formatting, and reports what it would rewrite
	// instead of rewriting it, which is the check to run in a pipeline.
	format := formatter.Paths
	if list {
		format = formatter.NeedFormatting
	}
	results, err := format(args)
	if err != nil {
		return err
	}
	for _, result := range results {
		switch {
		case result.Skipped != nil:
			fmt.Fprintf(errOut, "skipped %s\n", result.Skipped)
		case result.Changed:
			fmt.Fprintln(out, result.Path)
		}
	}
	return nil
}
