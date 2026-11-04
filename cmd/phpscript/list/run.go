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
	var bindings, classes, api, asJSON bool

	return &cli.Command{
		Name:  "list",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&bindings, "bindings", false, "List the functions, classes and constants the runtime registers instead of scanning paths")
			fs.BoolVar(&classes, "classes", false, "Print the registered classes as PHP class definitions, all of them or the ones named as arguments")
			fs.BoolVar(&api, "api", false, "List the methods of every class and interface, one row per method")
			fs.BoolVar(&asJSON, "json", false, "Write the rows as JSON instead of a markdown table")
		},
		Run: func(ctx context.Context, args []string) error {
			return Run(args, bindings, classes, api, asJSON)
		},
	}
}

// Run prints a markdown table for the paths, or for the runtime surface.
func Run(args []string, bindings, classes, api, asJSON bool) error {
	return run(os.Stdout, args, bindings, classes, api, asJSON)
}

func run(out io.Writer, args []string, bindings, classes, api, asJSON bool) error {
	// --bindings reports on the binary, and a tree or path argument selects no
	// file, and the command refuses it: a caller that passed both meant one of
	// the two.
	if bindings {
		if len(args) > 0 {
			return fmt.Errorf("list --bindings takes no path arguments, %d given", len(args))
		}
		if classes || api || asJSON {
			return fmt.Errorf("list --bindings takes none of --classes, --api or --json")
		}
		fmt.Fprint(out, phplist.StdlibMarkdown(phplist.Stdlib()))
		return nil
	}
	// --classes reports on the binary too, and its arguments are class names
	// and not paths. --json writes the class the definition is rendered from,
	// for a generator that links each member rather than printing a block of
	// PHP: both spellings of every symbol, the parameters, the citation of the
	// bound Go type and the interfaces it satisfies.
	if classes {
		if api {
			return fmt.Errorf("list --classes takes no --api")
		}
		if asJSON {
			defs, err := phplist.StdlibClasses(args...)
			if err != nil {
				return err
			}
			return writeJSON(out, defs)
		}
		defs, err := phplist.StdlibDefinitions(args...)
		if err != nil {
			return err
		}
		fmt.Fprint(out, defs)
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
