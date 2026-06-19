package run

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// Name is the command title.
const Name = "Run php script"

// NewCommand creates a new version command with build information.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args)
		},
	}
}

// Run runs the command with options and CLI arguments.
func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: phpscript <file.php>")
	}

	filename := args[0]
	buf, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("error reading %s: %w", filename, err)
	}

	prog, err := parser.Parse(string(buf))
	if err != nil {
		return fmt.Errorf("error parsing %s: %w", filename, err)
	}

	rt := runner.New(os.Stdout)
	rt.SetFS(os.DirFS("."), parser.Parse)

	stdlib.Register(rt)
	stdlib.RegisterFS(rt, ".")

	if err := rt.Run(prog); err != nil {
		return err
	}
	return nil
}
