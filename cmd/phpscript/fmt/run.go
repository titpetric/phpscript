package fmt

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/formatter"
	"github.com/titpetric/phpscript/internal/flags"
)

// Name is the command title.
const Name = "Format php scripts"

// NewCommand creates a new fmt command.
//
// Configuration and global options form the input every command constructor
// takes; one signature covers them all. This command reads neither.
func NewCommand(_ *config.Config, _ *flags.Options) *cli.Command {
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

// Run formats files or directories in place, reporting the files it left alone.
func Run(args []string, list bool) error {
	return run(args, list, os.Stdout, os.Stderr)
}

func run(args []string, list bool, out, errOut io.Writer) error {
	// Listing runs the same formatting. It reports the files it would rewrite
	// and rewrites nothing. That is the check to run in a pipeline.
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
