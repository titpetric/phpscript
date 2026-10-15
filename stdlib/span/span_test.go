package span_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/span"
)

// TestRegister covers the one name this package installs. A runtime nobody
// registered against answers for no binding, so a host that built its own
// Runtime without stdlib has to call Register for start_span to resolve.
func TestRegister(t *testing.T) {
	t.Parallel()
	rt := runner.New(&strings.Builder{}, runner.Options{})
	if rt.FunctionExists("start_span") {
		t.Fatal("a bare runtime answers for start_span before anything registered it")
	}

	span.Register(rt)
	if !rt.FunctionExists("start_span") {
		t.Error("start_span does not resolve after Register")
	}
}

// TestRegisterWithoutTrace covers a CLI run, where nothing opened a trace: the
// span start_span returns carries no recording and its methods do nothing, so
// a script instrumenting itself runs unchanged with telemetry switched off.
func TestRegisterWithoutTrace(t *testing.T) {
	t.Parallel()
	program, err := parser.Parse(`<?php
$span = start_span("getUser", "database");
$span->set_attribute("user_id", 42);
$span->end();
echo "ran";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	span.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); got != "ran" {
		t.Errorf("output = %q, want %q", got, "ran")
	}
}
