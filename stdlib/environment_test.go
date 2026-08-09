package stdlib_test

import (
	"os"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

func TestEnvironmentIsRuntimeScoped(t *testing.T) {
	const name = "PHPSCRIPT_ENV_TEST"
	t.Setenv(name, "host")

	run := func(source string) string {
		t.Helper()
		program, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		var out strings.Builder
		rt := runner.New(&out, runner.Options{})
		stdlib.Register(rt)
		if err := rt.Run(program); err != nil {
			t.Fatalf("run: %v", err)
		}
		return out.String()
	}

	if got := run(`<?php putenv("PHPSCRIPT_ENV_TEST", "request"); echo getenv("PHPSCRIPT_ENV_TEST");`); got != "request" {
		t.Fatalf("request environment = %q, want request", got)
	}
	if got := run(`<?php echo getenv("PHPSCRIPT_ENV_TEST");`); got != "host" {
		t.Fatalf("next request environment = %q, want host", got)
	}
	if got := os.Getenv(name); got != "host" {
		t.Fatalf("host environment mutated to %q", got)
	}
}

func TestPutenvSupportsAssignmentAndUnset(t *testing.T) {
	program, err := parser.Parse(`<?php
putenv("PHPSCRIPT_ASSIGNMENT=value");
echo getenv("PHPSCRIPT_ASSIGNMENT");
putenv("PHPSCRIPT_ASSIGNMENT");
echo getenv("PHPSCRIPT_ASSIGNMENT") === false ? "-missing" : "-present";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "value-missing"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
