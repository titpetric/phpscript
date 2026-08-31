package list

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/titpetric/cli"

	phplist "github.com/titpetric/phpscript/list"
)

// Name is the command title.
const Name = "List routes, files and classes"

// NewCommand creates a new list command.
func NewCommand() *cli.Command {
	var stdlib bool

	return &cli.Command{
		Name:  "list",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&stdlib, "stdlib", false, "List the functions, classes and constants the runtime registers instead of scanning paths")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(args, stdlib)
		},
	}
}

// Run prints a markdown table for the given path arguments, or for the
// registered runtime surface when stdlib is set.
func Run(args []string, stdlib bool) error {
	return run(os.Stdout, args, stdlib)
}

func run(out io.Writer, args []string, stdlib bool) error {
	// --stdlib asks about the binary, not about a tree, so path arguments
	// have nothing to select and are refused rather than ignored: a caller
	// that passed both meant one of the two.
	if stdlib {
		if len(args) > 0 {
			return fmt.Errorf("list --stdlib takes no path arguments, %d given", len(args))
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
