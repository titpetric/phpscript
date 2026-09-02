package flags_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner/coverage"
)

// TestPre covers the two flags read before a command exists. Both are taken
// off the argument list, because pflag runs after the configuration has been
// read and would report them as unknown.
func TestPre(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		file       string
		workdir    string
		remaining  []string
		wantErrArg string
	}{
		{
			name:      "separate values",
			args:      []string{"-f", "config.yml", "-w", "site", "server", "."},
			file:      "config.yml",
			workdir:   "site",
			remaining: []string{"server", "."},
		},
		{
			name:      "assigned values",
			args:      []string{"--file=config.yml", "--workdir=site", "test"},
			file:      "config.yml",
			workdir:   "site",
			remaining: []string{"test"},
		},
		{
			name:      "neither",
			args:      []string{"test", "./..."},
			remaining: []string{"test", "./..."},
		},
		{name: "value missing", args: []string{"server", "-f"}, wantErrArg: "-f"},
		{name: "empty assignment", args: []string{"--workdir="}, wantErrArg: "--workdir"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, remaining, err := flags.Pre(tc.args)
			if tc.wantErrArg != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrArg) {
					t.Fatalf("err = %v, want one naming %s", err, tc.wantErrArg)
				}
				return
			}
			if err != nil {
				t.Fatalf("Pre: %v", err)
			}
			if opts.ConfigFile != tc.file || opts.WorkDir != tc.workdir {
				t.Errorf("file = %q workdir = %q, want %q and %q", opts.ConfigFile, opts.WorkDir, tc.file, tc.workdir)
			}
			if strings.Join(remaining, " ") != strings.Join(tc.remaining, " ") {
				t.Errorf("remaining = %v, want %v", remaining, tc.remaining)
			}
		})
	}
}

// TestHoist covers the reordering that lets a global flag precede the command.
// The cli library reads the command out of the leading arguments and stops at
// the first flag, so without this `phpscript --cover server` runs a file called
// "server".
func TestHoist(t *testing.T) {
	isCommand := func(name string) bool { return name == "server" || name == "test" }
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "flag before the command",
			args: []string{"--cover", "--coverfile", "x.cov", "server", "."},
			want: []string{"server", "--cover", "--coverfile", "x.cov", "."},
		},
		{
			name: "assigned value",
			args: []string{"--cover=file", "test", "./..."},
			want: []string{"test", "--cover=file", "./..."},
		},
		{
			name: "value that names a command",
			args: []string{"--include", "server", "test"},
			want: []string{"test", "--include", "server"},
		},
		{
			name: "command already first",
			args: []string{"server", "--cover"},
			want: []string{"server", "--cover"},
		},
		{
			name: "no command at all",
			args: []string{"--cover", "app.php"},
			want: []string{"--cover", "app.php"},
		},
		{
			name: "a command's own flag stops the walk",
			args: []string{"--matrix", "test"},
			want: []string{"--matrix", "test"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := flags.Hoist(tc.args, isCommand)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("Hoist = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOptions_Covering pins which modes turn measurement on. Every command
// asks this before it installs a collector.
func TestOptions_Covering(t *testing.T) {
	for mode, want := range map[string]bool{
		"":                false,
		coverage.ModeLine: true,
		coverage.ModeFunc: true,
		coverage.ModeFile: true,
	} {
		if got := (&flags.Options{Cover: mode}).Covering(); got != want {
			t.Errorf("cover=%q Covering() = %v, want %v", mode, got, want)
		}
	}
}

// TestOptions_Report pins which modes own stdout with a per-symbol report on
// top of writing the profile. It is what rules out combining one with --json.
func TestOptions_Report(t *testing.T) {
	for mode, want := range map[string]bool{
		"":                false,
		coverage.ModeLine: false,
		coverage.ModeFunc: true,
		coverage.ModeFile: true,
	} {
		if got := (&flags.Options{Cover: mode}).Report(); got != want {
			t.Errorf("cover=%q Report() = %v, want %v", mode, got, want)
		}
	}
}

// TestOptions_BindWith pins that the shared set is declared before a command's
// own flags, and that a command binding none still carries it.
func TestOptions_BindWith(t *testing.T) {
	var opts flags.Options
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opts.BindWith(func(fs *pflag.FlagSet) { fs.Bool("matrix", false, "Run every runtime") })(fs)
	if fs.Lookup("cover") == nil || fs.Lookup("matrix") == nil {
		t.Error("BindWith dropped a flag")
	}

	bare := pflag.NewFlagSet("version", pflag.ContinueOnError)
	opts.BindWith(nil)(bare)
	if bare.Lookup("cover") == nil {
		t.Error("a command binding no flags of its own lost the shared set")
	}
}

// TestOptions_Bind pins that every shared flag is declared once, with the
// shorthands the documentation names.
func TestOptions_Bind(t *testing.T) {
	var opts flags.Options
	fs := pflag.NewFlagSet("phpscript", pflag.ContinueOnError)
	opts.Bind(fs)

	for name, shorthand := range map[string]string{
		"file": "f", "workdir": "w", "verbose": "v",
		"include": "", "cpuprofile": "", "memprofile": "", "cover": "", "coverfile": "",
	} {
		flag := fs.Lookup(name)
		if flag == nil {
			t.Fatalf("--%s is not declared", name)
		}
		if flag.Shorthand != shorthand {
			t.Errorf("--%s shorthand = %q, want %q", name, flag.Shorthand, shorthand)
		}
		if flag.Usage == "" {
			t.Errorf("--%s has no usage text", name)
		}
	}
	// A bare --cover is --cover=line, which is what makes it legal to write
	// without a value in front of a command name.
	if got := fs.Lookup("cover").NoOptDefVal; got != coverage.ModeLine {
		t.Errorf("--cover NoOptDefVal = %q, want %q", got, coverage.ModeLine)
	}
}

// TestOptions_Validate covers the coverage vocabulary every command shares.
func TestOptions_Validate(t *testing.T) {
	t.Run("coverfile implies cover", func(t *testing.T) {
		opts := flags.Options{CoverFile: "x.cov"}
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if opts.Cover != coverage.ModeLine {
			t.Errorf("Cover = %q, want %q", opts.Cover, coverage.ModeLine)
		}
	})
	t.Run("cover implies a coverfile", func(t *testing.T) {
		opts := flags.Options{Cover: coverage.ModeFunc}
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if opts.CoverFile != coverage.DefaultCoverFile {
			t.Errorf("CoverFile = %q, want %q", opts.CoverFile, coverage.DefaultCoverFile)
		}
		if !opts.Report() || !opts.Covering() {
			t.Errorf("Report = %v Covering = %v, want both true", opts.Report(), opts.Covering())
		}
	})
	t.Run("unknown mode", func(t *testing.T) {
		opts := flags.Options{Cover: "statements"}
		if err := opts.Validate(); err == nil || !strings.Contains(err.Error(), "statements") {
			t.Errorf("err = %v, want one naming the mode", err)
		}
	})
	t.Run("off", func(t *testing.T) {
		var opts flags.Options
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if opts.Covering() || opts.Report() || opts.CoverFile != "" {
			t.Errorf("options = %+v, want coverage off", opts)
		}
	})
}

// TestOptions_FromConfig pins the precedence: a configuration describes a tree and a
// flag is what an operator typed about this run of it, so the flag wins.
func TestOptions_FromConfig(t *testing.T) {
	appConfig := config.New()
	appConfig.Runner.Include = "vendor/autoload.php"

	var unset flags.Options
	unset.FromConfig(appConfig)
	if unset.Include != "vendor/autoload.php" {
		t.Errorf("include = %q, want the configured file", unset.Include)
	}

	named := flags.Options{Include: "boot.php"}
	named.FromConfig(appConfig)
	if named.Include != "boot.php" {
		t.Errorf("include = %q, want the flag to win", named.Include)
	}
}

// TestOptions_ResolveCoverFile covers the {time} placeholder and the directory it
// writes into, which is what makes cover/phpscript.{time}.cov work against a
// tree that has no cover directory.
func TestOptions_ResolveCoverFile(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 2, 14, 30, 5, 0, time.UTC)

	opts := flags.Options{CoverFile: filepath.Join(dir, "cover", "site.{time}.cov")}
	name, err := opts.ResolveCoverFile(at)
	if err != nil {
		t.Fatalf("ResolveCoverFile: %v", err)
	}
	if want := filepath.Join(dir, "cover", "site.20260902-143005.cov"); name != want {
		t.Errorf("name = %q, want %q", name, want)
	}
	if info, err := os.Stat(filepath.Dir(name)); err != nil || !info.IsDir() {
		t.Errorf("parent directory: err = %v", err)
	}

	empty := flags.Options{}
	if name, err := empty.ResolveCoverFile(at); err != nil || name != coverage.DefaultCoverFile {
		t.Errorf("unset coverfile = %q err = %v, want the default", name, err)
	}
}

// TestOptions_WriteCoverProfile covers the write path every command shares.
func TestOptions_WriteCoverProfile(t *testing.T) {
	dir := t.TempDir()
	opts := flags.Options{CoverFile: filepath.Join(dir, "out", "x.cov")}
	name, err := opts.WriteCoverProfile([]coverage.ProfileBlock{
		{File: "app.php", StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 20, NumStmt: 1, Count: 3},
	})
	if err != nil {
		t.Fatalf("WriteCoverProfile: %v", err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if got, want := string(data), "mode: count\napp.php:2.1,2.20 1 3\n"; got != want {
		t.Errorf("profile = %q, want %q", got, want)
	}
}

// TestOptions_RunWith covers the wrapper every command runs under: the flags validate,
// the profiles land, and the command's own error comes back unchanged.
func TestOptions_RunWith(t *testing.T) {
	dir := t.TempDir()
	cpu := filepath.Join(dir, "cpu.pprof")
	mem := filepath.Join(dir, "mem.pprof")

	opts := flags.Options{CPUProfile: cpu, MemProfile: mem}
	var ran bool
	run := opts.RunWith(func(context.Context, []string) error {
		ran = true
		return nil
	})
	if err := run(context.Background(), nil); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if !ran {
		t.Fatal("the wrapped command did not run")
	}
	for _, name := range []string{cpu, mem} {
		info, err := os.Stat(name)
		if err != nil {
			t.Errorf("profile %s: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("profile %s is empty", name)
		}
	}

	t.Run("command error", func(t *testing.T) {
		want := errors.New("boom")
		var opts flags.Options
		if err := opts.RunWith(func(context.Context, []string) error { return want })(context.Background(), nil); !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
	})

	t.Run("invalid flags do not run the command", func(t *testing.T) {
		opts := flags.Options{Cover: "statements"}
		err := opts.RunWith(func(context.Context, []string) error {
			t.Error("the command ran with an invalid cover mode")
			return nil
		})(context.Background(), nil)
		if err == nil {
			t.Error("err = nil, want a cover mode error")
		}
	})
}

// TestOptions_Chdir covers -w, which moves the process before the configuration is read
// so a tree is named once rather than in every path a configuration holds.
func TestOptions_Chdir(t *testing.T) {
	dir := t.TempDir()
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(before) })

	opts := flags.Options{WorkDir: dir}
	if err := opts.Chdir(); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	got, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// macOS hands out a symlinked temporary directory, so the comparison is on
	// the resolved path rather than the one handed in.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("cwd = %q, want %q", got, want)
	}

	if err := (&flags.Options{}).Chdir(); err != nil {
		t.Errorf("unset workdir: %v", err)
	}
	if err := (&flags.Options{WorkDir: filepath.Join(dir, "absent")}).Chdir(); err == nil {
		t.Error("err = nil, want a missing directory to fail the command")
	}
}
