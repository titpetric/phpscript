package lint

import (
	"context"
	"fmt"
	"os"

	"github.com/titpetric/cli"

	phplint "github.com/titpetric/phpscript/lint"
)

// Name is the command title.
const Name = "Lint php scripts"

// NewCommand creates a new lint command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "lint",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(args)
		},
	}
}

// Run lints files or directories and reports diagnostics.
func Run(args []string) error {
	diags, err := phplint.Paths(args)
	if err != nil {
		return err
	}
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d.String())
	}
	if len(diags) > 0 {
		return fmt.Errorf("lint failed with %d diagnostic(s)", len(diags))
	}
	return nil
}
