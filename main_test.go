package main

import (
	"os"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/ast"
	"github.com/titpetric/phpscript/cmd/phpscript/helpdocs"
	"github.com/titpetric/phpscript/cmd/phpscript/run"
	"github.com/titpetric/phpscript/cmd/phpscript/toolchain"
	"github.com/titpetric/phpscript/internal/flags"
)

// TestCommandsBindWithGlobals binds every command the way decorate does: the
// global flags first, then the command's own. pflag panics on a name declared
// twice on one set and a command that reuses a global's name fails here and
// never in a run of that command. A command package binding on its own flag
// set never meets the globals and cannot see the clash.
func TestCommandsBindWithGlobals(t *testing.T) {
	t.Parallel()
	for _, command := range commands() {
		t.Run(command.name, func(t *testing.T) {
			bind := (&flags.Options{}).BindWith(command.new(nil, nil).Bind)
			bind(helpdocs.NewFlagSet(command.name))
		})
	}
}

// TestWantsHelp covers which spellings main answers itself. main prints the
// whole document; a help flag written after a command name belongs to that
// command, and the library prints that command's usage. The split is
// positional and the first argument that is not a flag ends main's claim on
// the line.
func TestWantsHelp(t *testing.T) {
	t.Parallel()
	document := [][]string{
		{"-h"},
		{"--help"},
		{"help"},
		{"-v", "--help"},
		{"--config=x", "help"},
	}
	for _, args := range document {
		if !wantsHelp(args) {
			t.Errorf("wantsHelp(%v) = false, want the whole document", args)
		}
	}

	command := [][]string{
		{},
		{"run"},
		{"run", "--help"},
		{"test", "-h"},
		{"server", "help"},
		{"-v", "run", "--help"},
	}
	for _, args := range command {
		if wantsHelp(args) {
			t.Errorf("wantsHelp(%v) = true, want the command to answer it", args)
		}
	}
}

// TestWriteHelp covers the long help: the shared flags printed once, then every
// registered command with the flags it adds. A command's section repeats no
// shared flag, because the document names the shared set once.
func TestWriteHelp(t *testing.T) {
	t.Parallel()
	file, err := os.CreateTemp(t.TempDir(), "help")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	commands := []registration{
		{"run", run.Name, run.NewCommand},
		{"ast", ast.Name, ast.NewCommand},
	}
	if err := writeHelp(file, commands); err != nil {
		t.Fatalf("writeHelp: %v", err)
	}

	written, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	document := string(written)

	for _, want := range []string{"run", "ast", run.Name, ast.Name} {
		if !strings.Contains(document, want) {
			t.Errorf("the help does not name %q:\n%s", want, document)
		}
	}

	// A shared flag appears in the shared section and in no command's own.
	shared := "--file"
	if !strings.Contains(document, shared) {
		t.Fatalf("the help does not name the shared flag %q:\n%s", shared, document)
	}
	if got := strings.Count(document, shared); got != 1 {
		t.Errorf("the shared flag %q is printed %d times, want once", shared, got)
	}
}

// TestRunToolPrefix pins the ambiguity rule: the prefix is the name the binary
// runs as. phpscript runs no phpx-* tool and phpx runs no phpscript-* tool.
func TestRunToolPrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, body := range map[string]string{"phpscript-gen": "exit 4", "phpx-test": "exit 5", "phpscript-test": "exit 6"} {
		if err := os.WriteFile(dir+"/"+name, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		argv []string
		code int
		ok   bool
	}{
		{[]string{"/bin/phpscript", "gen"}, 4, true},
		{[]string{"/bin/phpscript", "test"}, 6, true},
		{[]string{"/bin/phpx", "test"}, 5, true},
		{[]string{"/bin/phpx", "gen"}, 0, false},
		{[]string{"/bin/phpscript", "run"}, 0, false},
		{[]string{"/bin/phpscript"}, 0, false},
	}
	for _, tc := range cases {
		code, ok := runTool(tc.argv, dir)
		if code != tc.code || ok != tc.ok {
			t.Errorf("runTool(%v) = %d, %v; want %d, %v", tc.argv, code, ok, tc.code, tc.ok)
		}
	}
}

// TestWithToolsReplacesBuiltin pins that a discovered test tool replaces the
// built-in and withTools appends a new tool.
func TestWithToolsReplacesBuiltin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"phpx-test", "phpx-gen"} {
		if err := os.WriteFile(dir+"/"+name, []byte("#!/bin/sh\necho \"Tool $0\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tools := toolchain.Discover("phpx", dir)
	got := withTools(commands(), "phpx", tools)
	if len(got) != len(commands())+1 {
		t.Fatalf("got %d commands, want built-ins plus gen", len(got))
	}
	titles := map[string]string{}
	for _, c := range got {
		titles[c.name] = c.title
	}
	if !strings.HasPrefix(titles["test"], "Tool ") || titles["gen"] == "" {
		t.Errorf("titles = %v", titles)
	}
}

// TestDecorate covers what a command gets for free. decorate binds the shared
// flags onto it and wraps its Run with the work they imply and no command
// package repeats either, and a command naming no usage takes the examples
// registered under its name.
func TestDecorate(t *testing.T) {
	t.Parallel()
	globals := &flags.Options{}
	command := decorate(nil, globals, registration{"run", run.Name, run.NewCommand})()

	if command.Bind == nil {
		t.Error("the decorated command binds nothing, so the shared flags are unreachable")
	}
	if command.Run == nil {
		t.Error("the decorated command has no Run")
	}
	if command.Usage == nil {
		t.Fatal("the decorated command states no usage")
	}
	if usage := command.Usage(); usage == "" {
		t.Error("the usage is empty, so `phpscript run --help` prints no examples")
	}
}
