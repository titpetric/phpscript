package config

import (
	"fmt"
)

// runKeys are the test block keys a phpscript.yml below the invocation root may
// not set. Each describes one run of the whole command rather than one folder of
// fixtures, and two folders answering differently would leave the run with no
// answer at all.
var runKeys = []string{"parallel", "cache", "skip_php"}

// runKeysOwner completes the sentence a rejected run key is reported with.
const runKeysOwner = "is set by the run, not by a suite"

// Test configures `phpscript test`. It is the defaults a fixture tree carries
// for itself, so a suite describes what it needs instead of the command line
// repeating it on every invocation. A flag still wins: the file describes a
// tree, and a flag is what an operator typed about this run of it.
//
// Include and Hooks belong to a suite root, the directory holding the file, and
// apply to the fixtures below it. Parallel, Cache and SkipPHP describe the whole
// run and are read only from the configuration -f names or the one found above
// the working directory; a file below the invocation root that sets one fails
// the run naming the key.
type Test struct {
	// Include names a file included ahead of every fixture below this suite
	// root, once per session, for the functions and classes the fixtures expect
	// to be there: a composer autoloader, a bootstrap file. It is the --include
	// flag, which overrides this. A file that is not there is skipped.
	Include string `yaml:"include"`

	// Hooks are the PHP files the session runs around the fixtures.
	Hooks Hooks `yaml:"hooks"`

	// Parallel is how many fixtures of one area may run at once, the
	// --parallel flag.
	Parallel int `yaml:"parallel"`

	// Cache is how far a parsed include and a compiled expression travel,
	// worker or off, the --cache flag.
	Cache string `yaml:"cache"`

	// SkipPHP leaves the php binary out of a matrix run, the --skip-php flag.
	SkipPHP bool `yaml:"skip_php"`
}

// Hooks are the PHP files a test session runs around the fixtures of one suite
// root. Both are resolved against that root, and both run once per session
// rather than once per fixture.
//
// They are configuration rather than an annotation because a test session is
// not a server. @startup, @route and @schedule are scanned out of a source tree
// by `phpscript server`, which runs them per virtual host or per application
// root depending on how it is configured; there is no such scope in a fixture
// run, and a comment in a file that a walk happened to reach is not a contract
// a suite can be held to.
type Hooks struct {
	// Setup runs once before any fixture below this suite root: the schema its
	// fixtures query and the rows they read. A failure fails the run before a
	// fixture executes, because a suite whose fixtures all fail on missing
	// state reports the same thing many times and names none of it.
	Setup string `yaml:"setup"`

	// Teardown runs once after those fixtures, whether they passed or failed,
	// so state a suite created outside its own database is still released by a
	// run that ended badly.
	Teardown string `yaml:"teardown"`
}

// TestSuite is one phpscript.yml governing a directory of fixtures: where it
// sits, and the configuration the fixtures below it run under.
type TestSuite struct {
	// Dir is the suite root, the directory holding the file. It is the
	// application root the fixtures below resolve their prelude and their hooks
	// against.
	Dir string

	// Config is the file read over the configuration the run started with.
	Config Config

	// Declared holds the keys the file itself named, so a caller can tell a
	// setting the suite asked for from one it inherited.
	Declared map[string]any
}

// LoadTestSuite reads a phpscript.yml already held as bytes over base. The
// caller does the reading because it owns the filesystem: a fixture tree is
// compiled into the test binary and reached through an fs.FS, while a run of
// the command reads the same file off disk.
//
// forbidden decides whether the run-wide keys are refused. The configuration
// found at or above the invocation root is the run's own and sets them; one
// discovered below it governs a folder and does not.
func LoadTestSuite(dir, filename string, data []byte, base Config, forbidden bool) (*TestSuite, error) {
	// The listen address and a nested site are the operator's here too: a suite
	// is a folder of fixtures, and neither key means anything to a run of them.
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

	if err := result.Test.Validate(filename); err != nil {
		return nil, err
	}
	return &TestSuite{Dir: dir, Config: result, Declared: declared}, nil
}

// Validate rejects a test block whose values name nothing the run can act on.
//
// Whether a hook file exists is not checked here, because the file lives in the
// caller's filesystem rather than on disk. The run reports a missing hook when
// it goes to execute it.
func (t Test) Validate(filename string) error {
	if t.Cache != "" && t.Cache != "worker" && t.Cache != "off" {
		return fmt.Errorf("%s: test.cache must be worker or off, not %q", filename, t.Cache)
	}
	if t.Parallel < 0 {
		return fmt.Errorf("%s: test.parallel must be at least 1, not %d", filename, t.Parallel)
	}
	return nil
}
