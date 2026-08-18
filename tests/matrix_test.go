package tests

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFixtureRunsMetadata(t *testing.T) {
	cases := []struct {
		name      string
		metadata  string
		flatstack bool
		php       bool
	}{
		{"default", "", true, true},
		{"php opted out", "runner:\n  php: false\n", true, false},
		{"flatstack opted out", "runner:\n  flatstack: false\n", false, true},
		{"both opted in", "runner:\n  flatstack: true\n  php: true\n", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "name: metadata\ndescription: runner metadata\n" + c.metadata + "---\n<?php echo \"ok\";\n---\nok"
			fx, err := ParseFixture([]byte(src), "metadata.phpt")
			if err != nil {
				t.Fatal(err)
			}
			if got := fx.Runs(RunnerFlatstack); got != c.flatstack {
				t.Errorf("Runs(flatstack) = %v, want %v", got, c.flatstack)
			}
			if got := fx.Runs(RunnerPHP); got != c.php {
				t.Errorf("Runs(php) = %v, want %v", got, c.php)
			}
			// The runtime defines the expected output, so it can not be
			// opted out of.
			if !fx.Runs(RunnerRuntime) {
				t.Error("Runs(runtime) = false, want true")
			}
		})
	}
}

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
