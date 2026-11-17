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
	"github.com/titpetric/phpscript/runner"
)

// Name is the command title.
const Name = "List routes, files and classes"

// NewCommand creates a new list command.
func NewCommand(cfg *config.Config, _ *flags.Options) *cli.Command {
	var bindings, classes, api, areas, asJSON bool
	var runtimeName string

	return &cli.Command{
		Name:  "list",
		Title: Name,
		Bind: func(fs *cli.FlagSet) {
			fs.BoolVar(&bindings, "bindings", false, "List the functions, classes and constants the runtime registers instead of scanning paths")
			fs.BoolVar(&areas, "areas", false, "List the binding areas, the Runtime\\PS, Runtime\\PX and Runtime\\Go namespaces, with what each defines; arguments narrow it to the namespaces named")
			fs.BoolVar(&classes, "classes", false, "Print the registered classes as PHP class definitions, all of them or the ones named as arguments")
			fs.BoolVar(&api, "api", false, "List the methods of every class and interface, one row per method")
			fs.BoolVar(&asJSON, "json", false, "Write the rows as JSON instead of a markdown table")
			fs.StringVar(&runtimeName, "runtime", "", "Report on this runtime's namespace, phpscript or go; defaults to the configured runner.runtime")
		},
		Run: func(ctx context.Context, args []string) error {
			options := cfg.Runner
			if runtimeName != "" {
				options.Runtime = runtimeName
			}
			if err := options.Check(); err != nil {
				return err
			}
			if areas {
				return runAreas(os.Stdout, options, args, bindings || classes || api, asJSON)
			}
			return runRuntime(os.Stdout, options, args, bindings, classes, api, asJSON)
		},
	}
}

// Run prints a markdown table for the paths, or for the default runtime surface.
func Run(args []string, bindings, classes, api, asJSON bool) error {
	return runRuntime(os.Stdout, runner.Options{}, args, bindings, classes, api, asJSON)
}

func run(out io.Writer, args []string, bindings, classes, api, asJSON bool) error {
	return runRuntime(out, runner.Options{}, args, bindings, classes, api, asJSON)
}

func runRuntime(out io.Writer, options runner.Options, args []string, bindings, classes, api, asJSON bool) error {
	// --bindings reports on the binary. A tree or path argument selects no file
	// and the command refuses it: a caller that passed both meant one of the two.
	if bindings {
		if len(args) > 0 {
			return fmt.Errorf("list --bindings takes no path arguments, %d given", len(args))
		}
		if classes || api {
			return fmt.Errorf("list --bindings takes none of --classes or --api")
		}
		symbols := phplist.Stdlib(options)
		if asJSON {
			return writeJSON(out, symbols)
		}
		fmt.Fprint(out, phplist.StdlibMarkdown(symbols))
		return nil
	}
	// --classes reports on the binary too. Its arguments are class names, never
	// paths. --json writes the class it renders the definition from, for a
	// generator that links each member in place of a block of PHP: both
	// spellings of every symbol, the parameters, the citation of the bound Go
	// type and the interfaces it satisfies.
	if classes {
		if api {
			return fmt.Errorf("list --classes takes no --api")
		}
		if asJSON {
			defs, err := phplist.StdlibClasses(options, args...)
			if err != nil {
				return err
			}
			return writeJSON(out, defs)
		}
		defs, err := phplist.StdlibDefinitions(options, args...)
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

// runAreas prints the binding areas as a markdown table, or as JSON. The
// namespace arguments narrow the rows; naming another listing mode alongside
// fails, since each mode answers one question.
func runAreas(out io.Writer, options runner.Options, args []string, mixed, asJSON bool) error {
	if mixed {
		return fmt.Errorf("list --areas takes none of --bindings, --classes or --api")
	}
	rows := phplist.Areas(options, args...)
	if asJSON {
		return writeJSON(out, rows)
	}
	fmt.Fprint(out, phplist.AreasMarkdown(rows))
	return nil
}
