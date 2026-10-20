package config_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestTest covers the block `phpscript test` reads out of `config.yml`. A fixture
// tree carries its defaults in a file, so a suite says what it needs and the
// command line is not repeating it on every invocation.
func TestTest(t *testing.T) {
	t.Parallel()
	block := config.New().Test

	if block.Parallel != 1 {
		t.Errorf("parallel = %d, want 1", block.Parallel)
	}
	if block.Cache != "worker" {
		t.Errorf("cache = %q, want worker", block.Cache)
	}
	if block.SkipPHP {
		t.Error("skip_php = true, the compiled defaults run the php column")
	}
	if block.Include != "" || block.Hooks.Setup != "" || block.Hooks.Teardown != "" {
		t.Errorf("block = %+v, want no prelude and no hooks", block)
	}
}

// TestTest_Validate covers the values a run can act on. Whether a hook file
// exists goes unchecked here: the file lives in the caller's filesystem, so the
// run reports a missing hook where it executes it.
func TestTest_Validate(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		block config.Test
		want  string
	}{
		{name: "the compiled defaults", block: config.New().Test},
		{name: "cache off", block: config.Test{Cache: "off", Parallel: 4}},
		{name: "no cache named", block: config.Test{Parallel: 0}},
		{name: "an unknown cache", block: config.Test{Cache: "request"}, want: "test.cache must be worker or off"},
		{name: "a negative parallel", block: config.Test{Parallel: -1}, want: "test.parallel must be at least 1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.block.Check("suite/phpscript.yml")
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatalf("%+v validated", test.block)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want it to carry %q", err.Error(), test.want)
			}
			if !strings.Contains(err.Error(), "suite/phpscript.yml") {
				t.Errorf("err = %q, want it to name the file", err.Error())
			}
		})
	}
}

// TestTestSuite covers the three things one phpscript.yml over a directory of
// fixtures records: the suite root, the configuration the fixtures below run
// under, and the keys the file itself named. A caller reads the last one to
// tell a setting the suite named from one it inherited.
func TestTestSuite(t *testing.T) {
	t.Parallel()
	suite, err := config.LoadTestSuite("suites/db", "suites/db/phpscript.yml",
		[]byte("test:\n  include: bootstrap.php\n"), config.New(), false)
	if err != nil {
		t.Fatal(err)
	}

	if suite.Dir != "suites/db" {
		t.Errorf("dir = %q, want suites/db", suite.Dir)
	}
	if suite.Config.Test.Include != "bootstrap.php" {
		t.Errorf("include = %q, want bootstrap.php", suite.Config.Test.Include)
	}
	if _, ok := suite.Declared["test"]; !ok {
		t.Errorf("declared = %+v, want it to name test", suite.Declared)
	}
	if _, ok := suite.Declared["routes"]; ok {
		t.Errorf("declared = %+v, want nothing for a key the file left out", suite.Declared)
	}
}

// TestLoadTestSuite covers the file a suite root carries. The caller does the
// reading because it owns the filesystem: the build compiles a fixture tree
// into the test binary and an fs.FS reaches it, while a run of the command
// reads the same file off disk.
func TestLoadTestSuite(t *testing.T) {
	t.Parallel()
	base := config.New()

	t.Run("a suite inherits what it does not name", func(t *testing.T) {
		suite, err := config.LoadTestSuite(".", "phpscript.yml",
			[]byte("test:\n  hooks:\n    setup: setup.php\n"), base, true)
		if err != nil {
			t.Fatal(err)
		}
		if suite.Config.Test.Hooks.Setup != "setup.php" {
			t.Errorf("setup = %q, want setup.php", suite.Config.Test.Hooks.Setup)
		}
		if suite.Config.Test.Cache != base.Test.Cache {
			t.Errorf("cache = %q, want %q", suite.Config.Test.Cache, base.Test.Cache)
		}
	})

	t.Run("a suite holds no sites", func(t *testing.T) {
		withSites := config.New()
		withSites.VirtualHost = []config.VirtualHost{{Domain: "one.test", Root: t.TempDir()}}

		suite, err := config.LoadTestSuite(".", "phpscript.yml", []byte("test:\n  parallel: 2\n"), withSites, false)
		if err != nil {
			t.Fatal(err)
		}
		if suite.Config.VirtualHost != nil {
			t.Errorf("virtualhost = %+v, want none", suite.Config.VirtualHost)
		}
	})

	t.Run("the run owns the run-wide keys", func(t *testing.T) {
		for _, key := range []string{"parallel", "cache", "skip_php"} {
			source := []byte("test:\n  " + key + ": " + runKeyValue(key) + "\n")

			// A file at or above the invocation root is the run's own and sets
			// them. One discovered below it governs a folder: two folders
			// answering differently would leave the run with no answer at all.
			if _, err := config.LoadTestSuite(".", "phpscript.yml", source, base, false); err != nil {
				t.Fatalf("%s from the run's own file: %v", key, err)
			}

			_, err := config.LoadTestSuite("suites/db", "suites/db/phpscript.yml", source, base, true)
			if err == nil {
				t.Fatalf("a suite below the invocation root set test.%s", key)
			}
			if !strings.Contains(err.Error(), "test."+key) {
				t.Errorf("err = %q, want it to name test.%s", err.Error(), key)
			}
		}
	})

	t.Run("the listen address is the operator's here too", func(t *testing.T) {
		source := []byte("server:\n  addr: \"127.0.0.1:9999\"\n")
		_, err := config.LoadTestSuite(".", "phpscript.yml", source, base, false)
		if err == nil {
			t.Fatal("a suite moved the listen address")
		}
		if !strings.Contains(err.Error(), "is set by the operator") {
			t.Errorf("err = %q", err.Error())
		}
	})

	t.Run("a bad test block fails the suite", func(t *testing.T) {
		source := []byte("test:\n  cache: request\n")
		if _, err := config.LoadTestSuite(".", "phpscript.yml", source, base, false); err == nil {
			t.Fatal("a suite naming an unknown cache loaded")
		}
	})

	t.Run("a bad mail block fails the suite", func(t *testing.T) {
		source := []byte("mail:\n  default:\n    from: noreply@example.com\n")
		_, err := config.LoadTestSuite(".", "phpscript.yml", source, base, false)
		if err == nil {
			t.Fatal("a suite naming a server with no host loaded")
		}
		if !strings.Contains(err.Error(), "host is required") {
			t.Errorf("err = %q", err.Error())
		}
	})
}

// runKeyValue is a legal value for each run-wide key, so the rejection a suite
// gets is over the key it named and not over the value.
func runKeyValue(key string) string {
	switch key {
	case "parallel":
		return "2"
	case "cache":
		return "off"
	default:
		return "true"
	}
}
