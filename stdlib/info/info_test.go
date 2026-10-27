package info_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/info"
)

// TestRegister covers the one name this package installs, and where its report
// lands: phpinfo() writes through the runtime's output and a buffered page
// captures it the way it captures echo.
func TestRegister(t *testing.T) {
	t.Parallel()
	program, err := parser.Parse(`<?php phpinfo();`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{SAPI: "cli"})
	if rt.FunctionExists("phpinfo") {
		t.Fatal("a bare runtime answers for phpinfo before anything registered it")
	}

	info.Register(rt)
	if !rt.FunctionExists("phpinfo") {
		t.Fatal("phpinfo does not resolve after Register")
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}

	report := out.String()
	for _, want := range []string{"phpscript", "Runtime => phpscript", "SAPI => cli", "Internal Functions => "} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not carry %q:\n%s", want, report)
		}
	}
}

// TestRegisterIgnoresArguments covers the signature PHP has and this binding
// does not read: phpinfo($what) selects sections in PHP, and the report here
// is one block and the binding takes an argument and changes nothing. A script
// written for PHP calls phpinfo(INFO_GENERAL) and has to keep running.
func TestRegisterIgnoresArguments(t *testing.T) {
	t.Parallel()
	program, err := parser.Parse(`<?php $ok = phpinfo(32); echo $ok ? "[true]" : "[false]";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{SAPI: "cli"})
	info.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasSuffix(out.String(), "[true]") {
		t.Errorf("phpinfo(32) did not return true:\n%s", out.String())
	}
}
