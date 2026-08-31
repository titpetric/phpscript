package tests

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/core"
)

func runPHP(t *testing.T, rt *runner.Runtime, source string) {
	t.Helper()
	program, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestDeferRunsAtFrameReturnInLIFOOrder(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	core.RegisterDefer(rt)

	runPHP(t, rt, `<?php
function work() {
    echo "body";
    defer(function() { echo "-first"; });
    defer(function() { echo "-second"; });
    return;
}
echo "before-";
work();
echo "-after";
defer(function() { echo "-file"; });`)

	if got, want := out.String(), "before-body-second-first-after-file"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

type closeRecorder struct {
	out *strings.Builder
}

func (r *closeRecorder) Close() {
	r.out.WriteString("-closed")
}

func TestDeferAcceptsNativeBoundMethodReference(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	core.RegisterDefer(rt)
	rt.RegisterConstructor("Resource", func() *closeRecorder {
		return &closeRecorder{out: &out}
	})

	runPHP(t, rt, `<?php
function work() {
    $resource = new Resource;
    defer($resource->close);
    echo "body";
}
work();
echo "-after";`)

	if got, want := out.String(), "body-closed-after"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDeferUsesIncludeFileBoundary(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	core.RegisterDefer(rt)
	rt.SetIncludeResolver(func(path string) (*model.Program, error) {
		return parser.Parse(`<?php
echo "child";
defer(function() { echo "-child-deferred"; });
return;`)
	})

	runPHP(t, rt, `<?php
defer(function() { echo "-main-deferred"; });
echo "main-";
include "child.php";
echo "-after";`)

	if got, want := out.String(), "main-child-child-deferred-after-main-deferred"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

type reentrantRecorder struct {
	out *strings.Builder
}

func (r *reentrantRecorder) Schedule(ctx context.Context) {
	r.out.WriteString("schedule-")
	scope, _ := runner.ScopeFromContext(ctx)
	scope.Defer(func() { r.out.WriteString("new-") })
}

func (r *reentrantRecorder) Old() {
	r.out.WriteString("old")
}

func TestDeferRegisteredDuringUnwindRunsWithoutReplacingOlderCallback(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	core.RegisterDefer(rt)
	rt.RegisterConstructor("Recorder", func() *reentrantRecorder {
		return &reentrantRecorder{out: &out}
	})

	runPHP(t, rt, `<?php
$recorder = new Recorder;
defer($recorder->old);
defer($recorder->schedule);`)

	if got, want := out.String(), "schedule-new-old"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDeferRejectsTypedNilCallback(t *testing.T) {
	program, err := parser.Parse(`<?php defer($callback);`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rt := runner.New(nil, runner.Options{})
	core.RegisterDefer(rt)
	var callback func()
	rt.SetGlobal("callback", callback)

	if err := rt.Run(program); err == nil || !strings.Contains(err.Error(), "argument must be callable") {
		t.Fatalf("run error = %v, want invalid callable error", err)
	}
}

func TestRegisterShutdownFunctionRunsInOrderWithMagicFile(t *testing.T) {
	var out strings.Builder
	root := fstest.MapFS{
		"main.php": {Data: []byte(`<?php
register_shutdown_function(function() { echo "-first:" . __FILE__; });
register_shutdown_function(function() { echo "-second:" . __FILE__; });
echo __FILE__ . "-body";
`)},
	}
	rt := runner.New(&out, runner.Options{RootFS: root})
	core.RegisterShutdown(rt)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "/main.php-body-first:/main.php-second:/main.php"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestShutdownClosureKeepsIncludedMagicFile(t *testing.T) {
	var out strings.Builder
	root := fstest.MapFS{
		"main.php": {Data: []byte(`<?php
echo __FILE__ . "-main";
include "bootstrap.php";
`)},
		"bootstrap.php": {Data: []byte(`<?php
echo "-" . __FILE__ . "-open";
register_shutdown_function(function() { echo "-" . __FILE__ . "-close"; });
`)},
	}
	rt := runner.New(&out, runner.Options{RootFS: root})
	core.RegisterShutdown(rt)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "/main.php-main-/bootstrap.php-open-/bootstrap.php-close"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
