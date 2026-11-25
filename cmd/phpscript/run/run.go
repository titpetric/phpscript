package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/ps/mail"
)

// Name is the command title.
const Name = "Run php script"

// NewCommand returns the run command on a copy of cfg. A later change to the
// configuration does not reach the run.
func NewCommand(cfg *config.Config, globals *flags.Options) *cli.Command {
	appConfig := config.Value(cfg)

	return &cli.Command{
		Name:  "run",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, appConfig, globals)
		},
	}
}

// Run runs the script once, and again for every SIGHUP: a reload cancels the
// generation running so a served script runs its shutdown callbacks and stops
// listening, then re-reads the file. SIGINT and SIGTERM end it for good.
func Run(ctx context.Context, args []string, config config.Config, globals *flags.Options) error {
	if len(args) == 0 {
		return errors.New("usage: phpscript <file.php>")
	}

	// The runtime resolves every path against RootFS, which cannot express a
	// path outside itself: cleanFSPath strips the leading slash and an
	// absolute path resolves against the working directory. Sandboxing is
	// the right default for an include inside a script, but not for the file
	// the user named on the command line. A script outside the working
	// directory roots the runtime at its directory instead.
	//
	// A path inside the working directory keeps today's behaviour, because PHP
	// resolves an include against the working directory and not the script
	// directory, and re-rooting every run would diverge from that.
	script, root, err := resolveEntrypoint(args[0])
	if err != nil {
		return err
	}

	for {
		generation, reloaded, stop := reloadScope(ctx)
		runErr := runOnce(generation, script, root, args, config, globals)
		stop()

		// A reload replaces the generation and its error together. The reload
		// ends the script; that ending is no failure of the run. Anything else
		// is the end of the run.
		if ctx.Err() != nil || !reloaded() {
			return runErr
		}
	}
}

// reloadScope returns a generation context and the reload test a SIGHUP sets.
func reloadScope(ctx context.Context) (context.Context, func() bool, func()) {
	generation, cancel := context.WithCancel(ctx)

	hangup := make(chan os.Signal, 1)
	signal.Notify(hangup, syscall.SIGHUP)

	var seen atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-hangup:
			seen.Store(true)
			cancel()
		case <-generation.Done():
		}
	}()

	stop := func() {
		signal.Stop(hangup)
		cancel()
		<-done
	}
	return generation, seen.Load, stop
}

// runOnce runs one generation from a fresh runtime, dropped on return.
func runOnce(ctx context.Context, script, root string, argv []string, config config.Config, globals *flags.Options) error {
	options := config.Runner
	if err := options.Check(); err != nil {
		return err
	}
	options.SAPI = "cli"
	options.RootFS = os.DirFS(root)
	options.Stdin = os.Stdin
	options.Include = globals.Include
	// A CLI run reads the process environment, plus the configuration's
	// additions. runner.ScriptEnvironment holds the infrastructure
	// variables back.
	options.Env = append(append([]string{}, os.Environ()...), config.Env...)
	// The mail servers the script can name are the ones the configuration
	// named. Without names, mail() and `new Mail` refuse catchably.
	options.Mail = mail.NewProvider(config.Mail)
	newRuntime := runner.New
	if config.Flatstack.Enabled {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(os.Stdout, options)
	// The signal context sends Ctrl-C and reload signals to the script. A
	// binding that blocks - a server parked in wait() - is waiting on this.
	rt.SetContext(ctx)

	// A collector turns flatstack off for this runtime: coverage is an
	// interpreter feature. The fallback is atomic. A counted program then runs
	// interpreted whole.
	var collector *coverage.Collector
	if globals.Covering() {
		collector = coverage.NewMode(globals.CoverageMode())
		rt.SetCoverage(collector)
	}

	prog, err := rt.LoadFile(script)
	if err != nil {
		return err
	}

	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)

	// The php CLI fills $argv the same way. It holds the script name from the
	// command line, plus the arguments after it.
	reqCtx := runner.NewContext()
	reqCtx.Argv = argv
	reqCtx.Register(rt)

	runErr := rt.Run(prog)
	// writeCoverage runs whatever the script ended with. A run that failed
	// halfway is exactly the run whose coverage is worth reading.
	if coverErr := writeCoverage(collector, globals, root); coverErr != nil {
		return coverErr
	}
	return runErr
}

// writeCoverage writes the profile and --cover report to stderr, outside the script's control.
func writeCoverage(collector *coverage.Collector, globals *flags.Options, root string) error {
	if collector == nil {
		return nil
	}
	source := func(file string) []string {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			return nil
		}
		return strings.Split(string(data), "\n")
	}
	blocks := coverage.Columns(collector.Blocks(), source)
	name, err := globals.WriteCoverProfile(blocks)
	if err != nil {
		return err
	}
	switch globals.Cover {
	case coverage.ModeFunc:
		return coverage.WriteReport(os.Stderr, coverage.FuncRows(blocks, collector.Functions(), collector.Files()))
	case coverage.ModeFile:
		return coverage.WriteReport(os.Stderr, coverage.FileRows(blocks, collector.Files()))
	default:
		if globals.Verbose {
			fmt.Fprintf(os.Stderr, "coverage: %.1f%% of statements, written to %s\n", coverage.Percent(blocks), name)
		}
	}
	return nil
}

// resolveEntrypoint returns the script path and root, the dir outside the workdir.
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
