package run

import (
	"context"
	"errors"
	"os"

	"github.com/titpetric/cli"

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

	rt := runner.New(os.Stdout, runner.Options{RootFS: os.DirFS(".")})
	prog, err := rt.LoadFile(args[0])
	if err != nil {
		return err
	}

	stdlib.Register(rt)
	stdlib.RegisterFS(rt, ".")

	if err := rt.Run(prog); err != nil {
		if exitErr, ok := runner.IsExit(err); ok {
			os.Exit(exitErr.Code)
		}
		return err
	}
	return nil
}
