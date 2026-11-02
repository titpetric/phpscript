package list

import (
	"context"
	"encoding/json"
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
	var bindings, api, asJSON bool

	return &cli.Command{
		Name:  "list",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&bindings, "bindings", false, "List the functions, classes and constants the runtime registers instead of scanning paths")
			fs.BoolVar(&api, "api", false, "List the methods of every class and interface, one row per method")
			fs.BoolVar(&asJSON, "json", false, "Write the rows as JSON instead of a markdown table")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(args, bindings, api, asJSON)
		},
	}
}

// Run prints a markdown table for the paths, or for the runtime surface.
func Run(args []string, bindings, api, asJSON bool) error {
	return run(os.Stdout, args, bindings, api, asJSON)
}

func run(out io.Writer, args []string, bindings, api, asJSON bool) error {
	// --bindings reports on the binary, and a tree or path argument selects no
	// file, and the command refuses it: a caller that passed both meant one of
	// the two.
	if bindings {
		if len(args) > 0 {
			return fmt.Errorf("list --bindings takes no path arguments, %d given", len(args))
		}
		if api || asJSON {
			return fmt.Errorf("list --bindings takes neither --api nor --json")
		}
		fmt.Fprint(out, phplist.StdlibMarkdown(phplist.Stdlib()))
		return nil
	}
	if api {
		rows, err := phplist.API(args)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(out, rows)
		}
		fmt.Fprint(out, phplist.APIMarkdown(rows))
		return nil
	}
	rows, err := phplist.Paths(args)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(out, rows)
	}
	fmt.Fprint(out, phplist.Markdown(rows))
	return nil
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
