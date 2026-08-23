package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/titpetric/cli"

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

	// The runtime resolves every path against RootFS, which cannot express a
	// path outside itself: cleanFSPath strips the leading slash, so an absolute
	// path would be looked up relative to the working directory. Sandboxing is
	// the right default for an include inside a script, but not for the file
	// the user named on the command line, so a script that sits outside the
	// working directory roots the runtime at its own directory instead.
	//
	// A path under the working directory keeps today's behaviour, because PHP
	// resolves an include against the working directory rather than the script
	// directory, and re-rooting every run would diverge from that.
	script, root, err := resolveEntrypoint(args[0])
	if err != nil {
		return err
	}

	options := config.Runner
	options.SAPI = "cli"
	options.RootFS = os.DirFS(root)
	options.Stdin = os.Stdin
	// A CLI run reads the process environment, with what the configuration
	// adds on top. runner.ScriptEnvironment holds the infrastructure
	// variables back.
	options.Env = append(append([]string{}, os.Environ()...), config.Env...)
	newRuntime := runner.New
	if config.Flatstack.Enabled {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(os.Stdout, options)
	prog, err := rt.LoadFile(script)
	if err != nil {
		return err
	}

	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)

	reqCtx := runner.NewContext()
	reqCtx.Register(rt)

	if err := rt.Run(prog); err != nil {
		if exitErr, ok := runner.IsExit(err); ok {
			os.Exit(exitErr.Code)
		}
		return err
	}
	return nil
}

// resolveEntrypoint splits the script the user named into the path to load and
// the directory to root the runtime at.
//
// A path at or below the working directory keeps the working directory as the
// root, so an include resolves the way PHP resolves it. Anything else, an
// absolute path or one that climbs out with "..", roots at the script's own
// directory, since the working directory cannot name it.
func resolveEntrypoint(arg string) (script, root string, err error) {
	absolute, err := filepath.Abs(arg)
	if err != nil {
		return "", "", fmt.Errorf("resolve %s: %w", arg, err)
	}
	workdir, err := filepath.Abs(".")
	if err != nil {
		return "", "", fmt.Errorf("resolve working directory: %w", err)
	}
	relative, err := filepath.Rel(workdir, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.Base(absolute), filepath.Dir(absolute), nil
	}
	return filepath.ToSlash(relative), ".", nil
}
