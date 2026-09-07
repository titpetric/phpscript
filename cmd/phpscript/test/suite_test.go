package test_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/test"
	"github.com/titpetric/phpscript/config"
)

// writeTree fills a temporary directory and makes it the working directory, so
// a run resolves the suite files the way an invocation from an application root
// does.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	tmp := t.TempDir()
	for name, content := range files {
		p := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(tmp)
	return tmp
}

const passingFixture = `name: passes
description: A fixture that asserts nothing about a suite.
---
<?php echo "ok";
---
ok
`

// TestSuiteIncludeIsAPreludeWithoutTheFlag covers the setting the whole feature
// exists for: a suite root names its prelude in its own file, and the fixtures
// below it load it with nothing on the command line.
func TestSuiteIncludeIsAPreludeWithoutTheFlag(t *testing.T) {
	writeTree(t, map[string]string{
		"phpscript.yml":   "test:\n  include: bootstrap.php\n",
		"bootstrap.php":   "<?php\n\nfunction app_name()\n{\n\treturn \"acme\";\n}\n",
		"suite/name.phpt": "name: prelude\ndescription: The suite's own file named the prelude.\n---\n<?php echo app_name();\n---\nacme\n",
	})

	if err := test.Run(context.Background(), []string{"suite"}, test.Options{Config: config.New()}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestSuiteHooksRunOncePerSession covers both hooks and the order they run in.
// Each appends to a file, so what is left records how many times each ran and
// which side of the fixtures it was on.
func TestSuiteHooksRunOncePerSession(t *testing.T) {
	tmp := writeTree(t, map[string]string{
		"suite/phpscript.yml": "test:\n  hooks:\n    setup: setup.php\n    teardown: teardown.php\n",
		"suite/setup.php":     "<?php file_put_contents(\"log.txt\", \"setup\\n\", FILE_APPEND);\n",
		"suite/teardown.php":  "<?php file_put_contents(\"log.txt\", \"teardown\\n\", FILE_APPEND);\n",
		"suite/a.phpt":        passingFixture,
		"suite/b.phpt":        strings.Replace(passingFixture, "name: passes", "name: passes too", 1),
	})

	if err := test.Run(context.Background(), []string{"suite"}, test.Options{Config: config.New()}); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, "suite", "log.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// Once each, whatever the fixture count, and the teardown last.
	if want := "setup\nteardown\n"; string(got) != want {
		t.Fatalf("hook log:\n got: %q\nwant: %q", got, want)
	}
}

// TestSuiteTeardownRunsAfterAFailure covers the half of the contract a passing
// run cannot show: a suite releases what it took whether or not the fixtures
// passed, and the run still reports the failure rather than the hook.
func TestSuiteTeardownRunsAfterAFailure(t *testing.T) {
	tmp := writeTree(t, map[string]string{
		"suite/phpscript.yml": "test:\n  hooks:\n    teardown: teardown.php\n",
		"suite/teardown.php":  "<?php file_put_contents(\"log.txt\", \"teardown\\n\");\n",
		"suite/fail.phpt":     "name: fails\ndescription: A fixture whose expected output is not what it prints.\n---\n<?php echo \"no\";\n---\nyes\n",
	})

	err := test.Run(context.Background(), []string{"suite"}, test.Options{Config: config.New()})
	if err == nil {
		t.Fatal("want the fixture failure reported")
	}
	if !strings.Contains(err.Error(), "fixture(s) failed") {
		t.Fatalf("want the fixture failure, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "suite", "log.txt")); err != nil {
		t.Fatalf("teardown did not run: %v", err)
	}
}

// TestSuiteSetupFailureStopsTheRun covers why a setup failure is fatal: the
// fixtures below it assert against state that was not laid down, and reporting
// each of them says the same thing many times and names none of it.
func TestSuiteSetupFailureStopsTheRun(t *testing.T) {
	tmp := writeTree(t, map[string]string{
		"suite/phpscript.yml": "test:\n  hooks:\n    setup: setup.php\n",
		"suite/setup.php":     "<?php throw new Exception(\"schema is not there\");\n",
		"suite/a.phpt":        strings.Replace(passingFixture, "echo \"ok\"", "file_put_contents(\"ran.txt\", \"x\"); echo \"ok\"", 1),
	})

	err := test.Run(context.Background(), []string{"suite"}, test.Options{Config: config.New()})
	if err == nil {
		t.Fatal("want the setup failure reported")
	}
	for _, want := range []string{"test setup", "setup.php", "schema is not there"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("want %q in the error, got %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "suite", "ran.txt")); err == nil {
		t.Fatal("a fixture ran after the setup failed")
	}
}

// TestSuiteEnvIsTheOnlyConnectionsBelowIt covers the replacement rule a virtual
// host is already held to: a suite that names its connections gets those and
// not the ones the run was started with.
func TestSuiteEnvIsTheOnlyConnectionsBelowIt(t *testing.T) {
	writeTree(t, map[string]string{
		"suite/phpscript.yml": "env:\n  - \"PLATFORM_DB_AREA=sqlite://:memory:\"\n",
		"suite/named.phpt": `name: the suite's own connection
runner:
  php: false
description: The connection the folder configured opens, and one it did not is refused.
---
<?php

$db = new Database("area");
$db->query("create table t (id integer primary key)");
echo "area ok\n";

try {
	new Database("sqlite_test");
} catch (Exception $e) {
	echo $e . "\n";
}
---
area ok
no configuration found for database: [sqlite_test]
`,
	})

	if err := test.Run(context.Background(), []string{"suite"}, test.Options{Config: config.New()}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestSuiteBelowTheRunRootMayNotSetRunKeys covers the tenant rule applied to a
// suite: --parallel describes one run of the command, so two folders cannot
// both answer for it.
func TestSuiteBelowTheRunRootMayNotSetRunKeys(t *testing.T) {
	writeTree(t, map[string]string{
		"suite/phpscript.yml": "test:\n  parallel: 4\n",
		"suite/a.phpt":        passingFixture,
	})

	err := test.Run(context.Background(), []string{"suite"}, test.Options{Config: config.New()})
	if err == nil {
		t.Fatal("want the run key refused")
	}
	if !strings.Contains(err.Error(), `"test.parallel" is set by the run, not by a suite`) {
		t.Fatalf("want the refusal to name the key, got %v", err)
	}
}

// TestRunSuiteSetsFlagDefaults covers the same keys where they are allowed: the
// file the run itself is under supplies them, and the run picks them up.
func TestRunSuiteSetsFlagDefaults(t *testing.T) {
	writeTree(t, map[string]string{
		"phpscript.yml": "test:\n  parallel: 4\n  cache: off\n",
		"suite/a.phpt":  passingFixture,
	})

	if err := test.Run(context.Background(), []string{"suite"}, test.Options{Config: config.New()}); err != nil {
		t.Fatalf("run: %v", err)
	}
}
