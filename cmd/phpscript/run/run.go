package run

import (
	"context"
	"errors"
	"os"
	"path"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/composer"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// Name is the command title.
const Name = "Run php script"

// NewCommand creates a new version command with build information.
func NewCommand(config config.Config) *cli.Command {
	return &cli.Command{
		Name:  "run",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, config)
		},
	}
}

// Run runs the command with options and CLI arguments.
func Run(ctx context.Context, args []string, config config.Config) error {
	if len(args) == 0 {
		return errors.New("usage: phpscript <file.php>")
	}

	options := config.Runner
	options.SAPI = "cli"
	options.RootFS = os.DirFS(".")
	options.Stdin = os.Stdin
	newRuntime := runner.New
	if config.Flatstack.Enabled {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(os.Stdout, options)
	prog, err := rt.LoadFile(args[0])
	if err != nil {
		return err
	}

	stdlib.Register(rt)
	stdlib.RegisterFS(rt, ".")

	reqCtx := runner.NewContext()
	reqCtx.Register(rt)

	// A composer project covering the script makes its vendor packages
	// resolvable without an include; scripts outside one are unaffected.
	if err := composer.Register(rt, options.RootFS, path.Dir(args[0])); err != nil {
		return err
	}

	if err := rt.Run(prog); err != nil {
		if exitErr, ok := runner.IsExit(err); ok {
			os.Exit(exitErr.Code)
		}
		return err
	}
	return nil
}
