package lint

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/titpetric/cli"

	phplint "github.com/titpetric/phpscript/lint"
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
	diags, err := phplint.Paths(args)
	if err != nil {
		return err
	}

	hasErrors := len(diags) > 0

	if checkFlatstack {
		fsDiags, err := phplint.FlatstackPaths(args)
		if err != nil {
			return err
		}
		diags = append(diags, fsDiags...)
		for _, d := range fsDiags {
			if strings.Contains(d.Message, "[flatstack unsupported]") {
				hasErrors = true
			}
		}
	}

	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d.String())
	}
	if hasErrors {
		return fmt.Errorf("lint completed with diagnostic findings or unsupported constructs")
	}
	return nil
}
