package config

import (
	"fmt"
)

// runKeys are test keys a suite below the invocation root cannot set.
var runKeys = []string{"parallel", "cache", "skip_php"}

// runKeysOwner completes the sentence that reports a rejected run key.
const runKeysOwner = "is set by the run, not by a suite"

// Test configures `phpscript test`. The block holds the defaults a fixture
// tree sets, plus a suite's needs. A command line never repeats them; flags
// win and describe this run.
//
// Include and Hooks belong to a suite root, the directory holding the file, and
// apply to the fixtures inside the suite root. Parallel, Cache and SkipPHP describe
// the whole run. The command takes them from the configuration -f names, or
// from the configuration found above the working directory alone; a file below
// the invocation root that sets one fails the run naming the key.
type Test struct {
	// Include names a file included ahead of every fixture below this suite
	// root, once per session, for the functions and classes the fixtures need:
	// a composer autoloader, a bootstrap file. The --include flag
	// overrides this key. The session skips a missing file.
	Include string `yaml:"include"`

	// Hooks are the PHP files the session runs around the fixtures.
	Hooks Hooks `yaml:"hooks"`

	// Parallel is how many fixtures of one area may run at once, the
	// --parallel flag.
	Parallel int `yaml:"parallel"`

	// Cache sets the travel of a parsed include and compiled expression,
	// worker or off, the --cache flag.
	Cache string `yaml:"cache"`

	// SkipPHP leaves the php binary out of a matrix run, the --skip-php flag.
	SkipPHP bool `yaml:"skip_php"`
}

// TestSuite covers a phpscript.yml governing a directory of fixtures: its
// location, and the configuration the fixtures in that directory run under.
type TestSuite struct {
	// Dir is the suite root, the directory holding the file, and the
	// application root the fixtures below resolve their prelude and their
	// hooks against.
	Dir string

	// Config is the file read over the configuration the run started with.
	Config Config

	// Declared holds the keys the file named. A caller can tell a suite-named
	// setting from an inherited one.
	Declared map[string]any
}

// LoadTestSuite reads a phpscript.yml already held as bytes over base. The
// caller does the reading because it owns the filesystem: the build compiles a
// fixture tree into the test binary and an fs.FS reaches it, while a run of
// the command reads the same file off disk.
//
// forbidden decides whether this suite may set the run-wide keys. A
// configuration found in the invocation root or a parent belongs to the run.
// It sets these keys. A configuration discovered below governs a folder and
// sets none.
func LoadTestSuite(dir, filename string, data []byte, base Config, forbidden bool) (*TestSuite, error) {
	// The listen address plus a nested site belong to the operator here as well. A
	// suite is a folder of fixtures; neither key means anything to a run of them.
	result, declared, err := OverlayBytes(base, filename, data, tenantKeys, tenantKeysOwner)
	if err != nil {
		return nil, err
	}

	// A suite holds no sites, whatever the file contained.
	result.VirtualHost = nil

	if forbidden {
		for _, key := range runKeys {
			if Declares(declared, "test", key) {
				return nil, fmt.Errorf("%s: %q %s", filename, "test."+key, runKeysOwner)
			}
		}
	}

	if err := result.Test.Check(filename); err != nil {
		return nil, err
	}
	if err := result.Mail.Check(filename); err != nil {
		return nil, err
	}
	return &TestSuite{Dir: dir, Config: result, Declared: declared}, nil
}

// Check rejects a test block whose values name nothing the run can act on.
// A hook file is not checked here; the run reports a missing one on execution.
func (t Test) Check(filename string) error {
	if t.Cache != "" && t.Cache != "worker" && t.Cache != "off" {
		return fmt.Errorf("%s: test.cache must be worker or off, not %q", filename, t.Cache)
	}
	if t.Parallel < 0 {
		return fmt.Errorf("%s: test.parallel must be at least 1, not %d", filename, t.Parallel)
	}
	return nil
}
