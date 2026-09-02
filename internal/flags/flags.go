// Package flags holds the command line options every phpscript subcommand
// accepts, and the work they imply.
//
// The set exists because the same idea used to be spelled once per command:
// --include in test and in lint with two help texts and two implementations
// and not in server at all, --cpuprofile only in test, -v defined twice, -f
// hand-parsed out of argv and printed by no help output. One Options is bound
// onto every command, and a command reads what it uses.
//
// -f and -w are read before the command is constructed, because the
// configuration file and the working directory decide what the command is
// handed. Pre takes them off the argument list; Bind declares them anyway, so
// they appear in help beside the rest.
package flags

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/runner/coverage"
)

// TimePlaceholder is the token --coverfile expands to a timestamp. A server
// writing one profile per run names them apart without the operator having to
// compose a filename per start.
const TimePlaceholder = "{time}"

// TimeLayout is what TimePlaceholder expands to: UTC, sortable, and free of
// the characters a shell or a path would have an opinion about.
const TimeLayout = "20060102-150405"

// Options are the flags every subcommand accepts.
type Options struct {
	// ConfigFile is -f: the configuration read over the embedded defaults.
	ConfigFile string

	// WorkDir is -w: the directory the process changes to before it does
	// anything else, so every relative path in the configuration, the
	// arguments and the scripts means the same thing.
	WorkDir string

	// Include is --include: a file included before the entrypoint, carried to
	// the runtime as runner.Options.Include and to the linter as its name
	// registry seed.
	Include string

	// Verbose is -v.
	Verbose bool

	// CPUProfile and MemProfile name pprof files written around the command.
	CPUProfile string
	MemProfile string

	// Cover is --cover: "", or one of the coverage modes. CoverFile is where
	// the profile is written, TimePlaceholder expanded.
	Cover     string
	CoverFile string
}

// Bind declares the shared flags on a command's flag set.
func (o *Options) Bind(fs *cli.FlagSet) {
	fs.StringVarP(&o.ConfigFile, "file", "f", o.ConfigFile, "Read this configuration file over the built-in defaults")
	fs.StringVarP(&o.WorkDir, "workdir", "w", o.WorkDir, "Change to this directory before running, so every relative path resolves below it")
	fs.StringVar(&o.Include, "include", o.Include, "Include this PHP file before the entrypoint, when it exists")
	fs.BoolVarP(&o.Verbose, "verbose", "v", o.Verbose, "Report more per command: fixture failures, bound names, startup detail")
	fs.StringVar(&o.CPUProfile, "cpuprofile", o.CPUProfile, "Write a pprof CPU profile of the command to this file")
	fs.StringVar(&o.MemProfile, "memprofile", o.MemProfile, "Write a pprof heap profile to this file when the command ends")
	fs.StringVar(&o.Cover, "cover", o.Cover, "Measure statement coverage: line writes the profile, func/file also print a coverage report")
	fs.Lookup("cover").NoOptDefVal = coverage.ModeLine
	fs.StringVar(&o.CoverFile, "coverfile", o.CoverFile, "Write the coverage profile to this file (implies --cover; default "+coverage.DefaultCoverFile+", "+TimePlaceholder+" expands to a timestamp)")
}

// BindWith returns a Bind that declares the shared flags before the command's
// own, so every command carries the set whether or not it defines flags.
func (o *Options) BindWith(bind func(*cli.FlagSet)) func(*cli.FlagSet) {
	return func(fs *cli.FlagSet) {
		o.Bind(fs)
		if bind != nil {
			bind(fs)
		}
	}
}

// RunWith wraps a command's Run with the shared work: the flags are validated,
// the CPU profile runs for the length of the command, and the heap profile is
// written after it. A failure to start profiling is the command's failure; the
// operator asked for a measurement and did not get one.
func (o *Options) RunWith(run func(context.Context, []string) error) func(context.Context, []string) error {
	return func(ctx context.Context, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		stop, err := o.startCPUProfile()
		if err != nil {
			return err
		}
		defer stop()
		if err := run(ctx, args); err != nil {
			return err
		}
		return o.writeMemProfile()
	}
}

// Pre reads -f and -w off the argument list and returns what is left.
//
// Both are read before the command is constructed: -f decides the
// configuration a command is handed, and -w decides what every relative path
// after it means, the configuration file included. pflag runs later and never
// sees them, which is why Bind's defaults are the values found here.
func Pre(args []string) (*Options, []string, error) {
	o := &Options{}
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var target *string
		switch {
		case arg == "-f" || arg == "--file":
			target = &o.ConfigFile
		case arg == "-w" || arg == "--workdir":
			target = &o.WorkDir
		case strings.HasPrefix(arg, "--file="):
			o.ConfigFile = strings.TrimPrefix(arg, "--file=")
			if o.ConfigFile == "" {
				return nil, nil, fmt.Errorf("--file requires a configuration file")
			}
			continue
		case strings.HasPrefix(arg, "--workdir="):
			o.WorkDir = strings.TrimPrefix(arg, "--workdir=")
			if o.WorkDir == "" {
				return nil, nil, fmt.Errorf("--workdir requires a directory")
			}
			continue
		default:
			remaining = append(remaining, arg)
			continue
		}
		if i+1 == len(args) {
			return nil, nil, fmt.Errorf("%s requires an argument", arg)
		}
		i++
		*target = args[i]
	}
	return o, remaining, nil
}

// valued names the shared flags that take a value, so a walk over the
// arguments knows which of them consume the word after them. --verbose is the
// only one that does not.
var valued = map[string]bool{
	"--include": true, "--cpuprofile": true, "--memprofile": true,
	"--cover": true, "--coverfile": true, "--file": true, "--workdir": true,
	"-f": true, "-w": true,
}

// Hoist moves the command name to the front of the argument list, so a global
// flag may precede it.
//
// Only the flags this package declares are walked over: a command's own flags
// belong after its name. A bare --cover does not consume the word after it,
// which is what its NoOptDefVal makes legal.
func Hoist(args []string, isCommand func(string) bool) []string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			if i == 0 || !isCommand(arg) {
				return args
			}
			hoisted := append([]string{arg}, args[:i]...)
			return append(hoisted, args[i+1:]...)
		}
		name, assigned := arg, false
		if at := strings.IndexByte(arg, '='); at >= 0 {
			name, assigned = arg[:at], true
		}
		if !valued[name] {
			// A flag this package does not declare belongs to a command, and a
			// command's flags follow its name. Nothing left to hoist.
			return args
		}
		if !assigned && name != "--cover" {
			i++
		}
	}
	return args
}

// Chdir applies -w. It runs before the configuration is read, so a tree is
// served by naming it once rather than by repeating it in every path the
// configuration holds.
func (o *Options) Chdir() error {
	if o.WorkDir == "" {
		return nil
	}
	if err := os.Chdir(o.WorkDir); err != nil {
		return fmt.Errorf("workdir %q: %w", o.WorkDir, err)
	}
	return nil
}

// FromConfig fills the options a configuration file may also carry, where the
// command line did not name one. The flag wins: a configuration describes a
// tree, and a flag is what an operator typed about this run of it.
func (o *Options) FromConfig(appConfig config.Config) {
	if o.Include == "" {
		o.Include = appConfig.Runner.Include
	}
}

// Validate normalizes the coverage flags and rejects a mode that is not one.
func (o *Options) Validate() error {
	if o.CoverFile != "" && o.Cover == "" {
		o.Cover = coverage.ModeLine
	}
	switch o.Cover {
	case "", coverage.ModeLine, coverage.ModeFunc, coverage.ModeFile:
	default:
		return fmt.Errorf("--cover: unknown mode %q, want %s, %s or %s", o.Cover, coverage.ModeLine, coverage.ModeFunc, coverage.ModeFile)
	}
	if o.Cover != "" && o.CoverFile == "" {
		o.CoverFile = coverage.DefaultCoverFile
	}
	return nil
}

// Covering reports whether coverage was asked for.
func (o *Options) Covering() bool { return o.Cover != "" }

// Report reports whether the coverage mode prints a per-symbol report on top
// of writing the profile.
func (o *Options) Report() bool {
	return o.Cover == coverage.ModeFunc || o.Cover == coverage.ModeFile
}

// ResolveCoverFile expands TimePlaceholder and creates the directory the
// profile is written into, so --coverfile=cover/phpscript.{time}.cov works
// against a tree that has no cover directory yet.
func (o *Options) ResolveCoverFile(now time.Time) (string, error) {
	name := o.CoverFile
	if name == "" {
		name = coverage.DefaultCoverFile
	}
	name = strings.ReplaceAll(name, TimePlaceholder, now.UTC().Format(TimeLayout))
	if dir := filepath.Dir(name); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("coverfile %q: %w", name, err)
		}
	}
	return name, nil
}

// WriteCoverProfile writes blocks to the resolved --coverfile.
func (o *Options) WriteCoverProfile(blocks []coverage.ProfileBlock) (string, error) {
	name, err := o.ResolveCoverFile(time.Now())
	if err != nil {
		return "", err
	}
	f, err := os.Create(name)
	if err != nil {
		return "", fmt.Errorf("create coverfile: %w", err)
	}
	defer f.Close()
	if err := coverage.WriteProfile(f, blocks); err != nil {
		return "", err
	}
	return name, nil
}

// startCPUProfile begins the CPU profile and returns the function that ends
// it. With no --cpuprofile the returned function does nothing.
func (o *Options) startCPUProfile() (func(), error) {
	if o.CPUProfile == "" {
		return func() {}, nil
	}
	f, err := os.Create(o.CPUProfile)
	if err != nil {
		return nil, fmt.Errorf("create cpuprofile: %w", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("start cpuprofile: %w", err)
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}, nil
}

// writeMemProfile writes the heap profile --memprofile named. The collection
// runs first, so the profile describes what is still reachable rather than
// what has not been swept yet.
func (o *Options) writeMemProfile() error {
	if o.MemProfile == "" {
		return nil
	}
	f, err := os.Create(o.MemProfile)
	if err != nil {
		return fmt.Errorf("create memprofile: %w", err)
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("write memprofile: %w", err)
	}
	return nil
}
