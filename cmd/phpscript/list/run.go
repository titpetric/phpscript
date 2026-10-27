package list

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	phplist "github.com/titpetric/phpscript/list"
)

// Name is the command title.
const Name = "List routes, files and classes"

// NewCommand creates a new list command.
//
// The configuration and the global options are what every command constructor
// takes and one signature covers them all. This command reads neither.
func NewCommand(_ *config.Config, _ *flags.Options) *cli.Command {
	var bindings bool

	return &cli.Command{
		Name:  "list",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&bindings, "bindings", false, "List the functions, classes and constants the runtime registers instead of scanning paths")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(args, bindings)
		},
	}
}

// Run prints a markdown table for the paths, or for the runtime surface.
func Run(args []string, bindings bool) error {
	return run(os.Stdout, args, bindings)
}

func run(out io.Writer, args []string, bindings bool) error {
	// --bindings reports on the binary, and a tree or path argument selects no
	// file, and the command refuses it: a caller that passed both meant one of
	// the two.
	if bindings {
		if len(args) > 0 {
			return fmt.Errorf("list --bindings takes no path arguments, %d given", len(args))
		}
		fmt.Fprint(out, phplist.StdlibMarkdown(phplist.Stdlib()))
		return nil
	}
	rows, err := phplist.Paths(args)
	if err != nil {
		return err
	}
	fmt.Fprint(out, phplist.Markdown(rows))
	return nil
}
