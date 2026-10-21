package tests

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRunFixtureOnPHP(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("php binary is not installed")
	}
	fx, err := ParseFixture([]byte("name: php\ndescription: php binary run\nstdin: \"in\"\nrequest:\n  args: [\"one\"]\n---\n<?php\necho $argv[1] . stream_get_contents(STDIN);\nexit(3);\n---\nonein"), "fixtures/php.phpt")
	if err != nil {
		t.Fatal(err)
	}
	// exit(3) picks the process status, which is not a failure on its own.
	res := RunFixtureOn(t.Context(), fx, RunnerPHP)
	if !res.Passed {
		t.Fatalf("fixture failed: %s", res.FailureReason)
	}
	if res.Runner != RunnerPHP {
		t.Errorf("runner = %q, want %q", res.Runner, RunnerPHP)
	}
}

func TestRunFixtureOnPHPReportsFatal(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("php binary is not installed")
	}
	fx, err := ParseFixture([]byte("name: php fatal\ndescription: php binary failure\n---\n<?php\nthrow new Exception(\"boom\");\n---\nunreachable"), "fixtures/php_fatal.phpt")
	if err != nil {
		t.Fatal(err)
	}
	res := RunFixtureOn(t.Context(), fx, RunnerPHP)
	if res.Passed {
		t.Fatal("fixture passed, want the fatal error reported")
	}
	if !strings.Contains(res.FailureReason, "boom") {
		t.Errorf("failure reason = %q, want it to name the php error", res.FailureReason)
	}
	// The throwaway copy php ran is not what a reader has in front of them.
	if strings.Contains(res.FailureReason, ".php_fatal.") {
		t.Errorf("failure reason names the temporary script: %q", res.FailureReason)
	}
}

func TestPHPFatalIgnoresWarnings(t *testing.T) {
	if err := phpFatal("PHP Warning:  something in file on line 1\n"); err != nil {
		t.Errorf("phpFatal(warning) = %v, want nil", err)
	}
	if err := phpFatal("PHP Fatal error:  Uncaught Error: boom\n"); err == nil {
		t.Error("phpFatal(fatal) = nil, want an error")
	}
}
