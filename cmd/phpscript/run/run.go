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
	"github.com/titpetric/phpscript/stdlib/mail"
)

// Name is the command title.
const Name = "Run php script"

// NewCommand creates a new run command.
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

// Run runs the command with options and CLI arguments.
//
// It runs the script once, and again for every SIGHUP: a reload ends the
// generation that is running, by cancelling the context it was given, and
// starts another that re-reads the file. A script that serves - one parked in
// HTTP\Server::wait - sees its context end, returns, and runs its shutdown
// callbacks, which is how it stops listening before the next generation binds.
//
// SIGINT and SIGTERM end it for good. Both arrive through ctx, which the caller
// installed, and reach the script because the runtime is given it: without that
// a served script ignores Ctrl-C, having nothing that notices.
func Run(ctx context.Context, args []string, config config.Config, globals *flags.Options) error {
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

	for {
		generation, reloaded, stop := reloadScope(ctx)
		runErr := runOnce(generation, script, root, config, globals)
		stop()

		// A reload replaces the generation and its error with it: the script
		// was ended on purpose, and what it ended with is not a failure of the
		// run. Anything else is the end of the run.
		if ctx.Err() != nil || !reloaded() {
			return runErr
		}
	}
}

// reloadScope derives the context one generation runs under, and answers
// whether a SIGHUP ended it.
//
// The signal cancels the generation rather than the run, so the script stops
// the way it stops for anything else - its context ends - and the loop above
// decides whether that was the last one.
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

// runOnce is one generation: a runtime, the script read and parsed again, and
// the run. Everything it builds is dropped when it returns, which is what makes
// a reload pick up an edited file.
func runOnce(ctx context.Context, script, root string, config config.Config, globals *flags.Options) error {
	options := config.Runner
	options.SAPI = "cli"
	options.RootFS = os.DirFS(root)
	options.Stdin = os.Stdin
	options.Include = globals.Include
	// A CLI run reads the process environment, with what the configuration
	// adds on top. runner.ScriptEnvironment holds the infrastructure
	// variables back.
	options.Env = append(append([]string{}, os.Environ()...), config.Env...)
	// The mail servers the script can name are the ones the configuration
	// named. With none, mail() and `new Mail` refuse catchably.
	options.Mail = mail.NewProvider(config.Mail)
	newRuntime := runner.New
	if config.Flatstack.Enabled {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(os.Stdout, options)
	// The signal context, so that Ctrl-C and a reload reach the script. A
	// binding that blocks - a server parked in wait() - is waiting on this.
	rt.SetContext(ctx)

	// A collector turns flatstack off for this runtime: coverage is an
	// interpreter feature and the fallback is atomic, so a counted program runs
	// interpreted whole.
	var collector *coverage.Collector
	if globals.Covering() {
		collector = coverage.New()
		rt.SetCoverage(collector)
	}

	prog, err := rt.LoadFile(script)
	if err != nil {
		return err
	}

	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)

	reqCtx := runner.NewContext()
	reqCtx.Register(rt)

	runErr := rt.Run(prog)
	// The profile is written whatever the script ended with. A run that failed
	// halfway is exactly the run whose coverage is worth reading.
	if coverErr := writeCoverage(collector, globals, root); coverErr != nil {
		return coverErr
	}
	if runErr != nil {
		if exitErr, ok := runner.IsExit(runErr); ok {
			os.Exit(exitErr.Code)
		}
		return runErr
	}
	return nil
}

// writeCoverage writes the profile the run collected, and the per-symbol report
// when --cover asked for one. Columns come from the source text below root, the
// way the profile format wants them.
//
// The report goes to stderr because the script owns stdout: a run pipes its own
// echo somewhere, and a coverage table mixed into it is corruption of the
// output the command exists to produce. `phpscript test` prints its report on
// stdout, because there the runner owns it.
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
