package runner_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

func TestExitInterruptsExecutionWithStatus(t *testing.T) {
	prog, err := parser.Parse(`<?php echo "before"; exit(7); echo "after";`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)

	err = rt.Run(prog)
	if err == nil {
		t.Fatal("expected exit error")
	}
	exitErr, ok := runner.IsExit(err)
	if !ok {
		t.Fatalf("expected exit error, got %v", err)
	}
	if exitErr.Code != 7 {
		t.Fatalf("exit code = %d, want 7", exitErr.Code)
	}
	if got, want := out.String(), "before"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestBareDieInterruptsExecution(t *testing.T) {
	prog, err := parser.Parse(`<?php echo "before"; die; echo "after";`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)

	err = rt.Run(prog)
	if err == nil {
		t.Fatal("expected exit error")
	}
	exitErr, ok := runner.IsExit(err)
	if !ok {
		t.Fatalf("expected exit error, got %v", err)
	}
	if exitErr.Code != 0 {
		t.Fatalf("exit code = %d, want 0", exitErr.Code)
	}
	if got, want := out.String(), "before"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
