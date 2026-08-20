package internals_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

func runScript(t *testing.T, src string, opts runner.Options) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, opts)
	stdlib.Register(rt)
	err = rt.Run(prog)
	return out.String(), err
}

func TestMemoryGetUsage(t *testing.T) {
	src := `<?php
$m0 = memory_get_usage();
$large = "hello world this is a test string allocated in the frame";
$m1 = memory_get_usage();
unset($large);
$m2 = memory_get_usage();
if ($m1 > $m0 && $m2 < $m1) {
    echo "ok";
} else {
    echo "fail: m0=" . $m0 . " m1=" . $m1 . " m2=" . $m2;
}
`
	out, err := runScript(t, src, runner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected ok, got: %s", out)
	}
}

// TestMemoryLimitExhaustion grows an array in place; the walk sees the
// growth even though no variable is reassigned, and the limit error is
// catchable as RuntimeException.
func TestMemoryLimitExhaustion(t *testing.T) {
	src := `<?php
try {
    $a = [];
    for ($i = 0; $i < 100000; $i++) {
        $a[] = str_repeat("x", 1000);
    }
    echo "unreachable";
} catch (RuntimeException $e) {
    echo "caught_runtime: " . get_class($e);
}
`
	limit, _ := runner.ParseSize("1M")
	out, err := runScript(t, src, runner.Options{MemoryLimit: limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "caught_runtime: RuntimeException" {
		t.Fatalf("expected 'caught_runtime: RuntimeException', got %q", out)
	}
}

func TestExceptionCatchHierarchy(t *testing.T) {
	src := `<?php
try {
    $s = "";
    for ($i = 0; $i < 100000; $i++) {
        $s = $s . str_repeat("y", 1000);
    }
    echo "unreachable";
} catch (Exception $e) {
    echo "caught_by_exception_base: " . get_class($e);
}
`
	limit, _ := runner.ParseSize("1M")
	out, err := runScript(t, src, runner.Options{MemoryLimit: limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "caught_by_exception_base: RuntimeException" {
		t.Fatalf("expected 'caught_by_exception_base: RuntimeException', got %q", out)
	}
}

// TestMemoryLimitUncaught exhausts the limit outside a try block; the error
// surfaces from Run.
func TestMemoryLimitUncaught(t *testing.T) {
	src := `<?php
$a = [];
for ($i = 0; $i < 100000; $i++) {
    $a[] = str_repeat("x", 1000);
}
echo "unreachable";
`
	limit, _ := runner.ParseSize("1M")
	out, err := runScript(t, src, runner.Options{MemoryLimit: limit})
	if err == nil {
		t.Fatalf("expected a memory limit error, got output %q", out)
	}
	if !strings.Contains(err.Error(), "Allowed memory size") {
		t.Fatalf("expected an exhaustion message, got: %v", err)
	}
}

// TestFrameRelease: a returned function's locals are no longer roots, so
// usage falls back to its pre-call level instead of leaking per call.
func TestFrameRelease(t *testing.T) {
	src := `<?php
function eat() {
    $x = str_repeat("a", 100000);
    return 1;
}
$m0 = memory_get_usage();
for ($i = 0; $i < 100; $i++) {
    eat();
}
$m1 = memory_get_usage();
echo ($m1 - $m0 < 10000) ? "ok" : "leaked: " . ($m1 - $m0);
`
	out, err := runScript(t, src, runner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected ok, got: %s", out)
	}
}

// TestAliasCountedOnce: $b = $a shares one array; the walk dedups it.
func TestAliasCountedOnce(t *testing.T) {
	src := `<?php
$a = [];
for ($i = 0; $i < 100; $i++) {
    $a[] = str_repeat("z", 1000);
}
$u1 = memory_get_usage();
$b = $a;
$u2 = memory_get_usage();
echo ($u2 - $u1 < 1000) ? "ok" : "double counted: " . ($u2 - $u1);
`
	out, err := runScript(t, src, runner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected ok, got: %s", out)
	}
}

// TestMemoryPeak: the peak survives an unset that drops current usage.
func TestMemoryPeak(t *testing.T) {
	src := `<?php
$big = str_repeat("c", 100000);
$u = memory_get_usage();
unset($big);
$peak = memory_get_peak_usage();
$now = memory_get_usage();
echo ($peak >= $u && $now < $u) ? "ok" : "peak=$peak u=$u now=$now";
`
	out, err := runScript(t, src, runner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected ok, got: %s", out)
	}
}

func TestCatchMultiType(t *testing.T) {
	src := `<?php
try {
    throw new RuntimeException("custom boom", 42);
} catch (InvalidArgumentException|RuntimeException $e) {
    echo "caught_multi: " . $e->getMessage() . " class: " . get_class($e);
}
`
	out, err := runScript(t, src, runner.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "caught_multi: custom boom class: RuntimeException" {
		t.Fatalf("expected 'caught_multi: custom boom class: RuntimeException', got %q", out)
	}
}
