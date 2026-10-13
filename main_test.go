package main

import (
	"os"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/ast"
	"github.com/titpetric/phpscript/cmd/phpscript/helpdocs"
	"github.com/titpetric/phpscript/cmd/phpscript/run"
	"github.com/titpetric/phpscript/internal/flags"
)

// TestCommandsBindWithGlobals binds every command the way decorate does: the
// global flags first, then the command's own. pflag panics on a name declared
// twice on one set, so a command that reuses a global's name fails here
// instead of panicking in every run of that command. A command package
// binding on a flag set of its own never meets the globals and cannot see the
// clash.
func TestCommandsBindWithGlobals(t *testing.T) {
	for _, command := range commands() {
		t.Run(command.name, func(t *testing.T) {
			bind := (&flags.Options{}).BindWith(command.new(nil, nil).Bind)
			bind(helpdocs.NewFlagSet(command.name))
		})
	}
}

// TestWantsHelp covers which spellings main answers itself. The whole document
// is main's to print; a help flag written after a command name belongs to that
// command, and the library prints its usage instead. The split is positional,
// so the first argument that is not a flag ends main's claim on the line.
func TestWantsHelp(t *testing.T) {
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
// registered command with the flags it adds of its own. A command's section
// repeats no shared flag, because the whole point of the shared set is that it
// is stated once.
func TestWriteHelp(t *testing.T) {
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

	// A shared flag is printed in the shared section and in no command's own.
	shared := "--file"
	if !strings.Contains(document, shared) {
		t.Fatalf("the help does not name the shared flag %q:\n%s", shared, document)
	}
	if got := strings.Count(document, shared); got != 1 {
		t.Errorf("the shared flag %q is printed %d times, want once", shared, got)
	}
}

// TestDecorate covers what a command gets for free. The shared flags are bound
// onto it and the work they imply wraps its Run, so no command package repeats
// either, and a command naming no usage of its own is given the examples
// registered under its name.
func TestDecorate(t *testing.T) {
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
