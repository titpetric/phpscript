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

func TestMemoryLimitExhaustion(t *testing.T) {
	src := `<?php
try {
    $a = "this string is definitely longer than twenty bytes limit";
    echo "unreachable";
} catch (RuntimeException $e) {
    echo "caught_runtime: " . get_class($e);
}
`
	limit, _ := runner.ParseSize("100") // 100 bytes limit
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
    $a = "long string to exceed limit";
} catch (Exception $e) {
    echo "caught_by_exception_base: " . get_class($e);
}
`
	limit, _ := runner.ParseSize("100")
	out, err := runScript(t, src, runner.Options{MemoryLimit: limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "caught_by_exception_base: RuntimeException" {
		t.Fatalf("expected 'caught_by_exception_base: RuntimeException', got %q", out)
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
