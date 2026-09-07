package test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/tests"
)

// suites are the phpscript.yml files one invocation runs under, ordered
// outermost first, with the run's own configuration at the front.
//
// A file marks a suite root: the directory whose fixtures resolve their
// prelude, their connections and their hooks through it. The run's own is
// whatever -f named or the nearest one above the working directory, and it is
// the only one allowed to set the keys that describe a whole run.
type suites struct {
	// run is the suite the invocation itself is under, or nil. Its directory
	// is not necessarily below the working directory, so it is kept apart from
	// the discovered ones and applies to every fixture that finds no closer
	// file.
	run *tests.Suite

	// found are the files discovered below the argument paths, sorted by
	// directory so an outer suite is set up before one nested inside it.
	found []*tests.Suite
}

// all returns every suite in setup order.
func (s suites) all() []*tests.Suite {
	result := make([]*tests.Suite, 0, len(s.found)+1)
	if s.run != nil {
		result = append(result, s.run)
	}
	return append(result, s.found...)
}

// resolve answers the suite governing a fixture: the nearest file at or above
// the directory holding it, and the run's own where no file is closer.
//
// Nearest wins because a suite is what a folder says about itself. A tree that
// configures a bootstrap at its root and a database in one folder below it
// gives that folder the database, not both, which is the same replacement rule
// a virtual host reads its own file under.
func (s suites) resolve(path string) *tests.Suite {
	dir := filepath.Dir(path)
	for {
		for _, suite := range s.found {
			if sameDir(suite.Dir, dir) {
				return suite
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return s.run
		}
		dir = parent
	}
}

// discoverSuites reads the configuration the run is under and every
// phpscript.yml below the argument paths.
//
// Both halves are read whether or not a fixture below them was selected, so a
// narrower path argument does not silently skip a suite's setup. The walk is
// the fixture walk: a bare directory is not recursive, and `./...` is what
// reaches a tree.
func discoverSuites(paths []string, base config.Config) (suites, error) {
	var result suites

	run, err := runSuite(base)
	if err != nil {
		return result, err
	}
	result.run = run

	seen := map[string]bool{}
	if run != nil {
		seen[absDir(run.Dir)] = true
	}

	load := func(dir string) error {
		key := absDir(dir)
		if seen[key] {
			return nil
		}
		suite, err := tests.LoadSuite(dir, base, true)
		if err != nil {
			return err
		}
		seen[key] = true
		if suite != nil {
			result.found = append(result.found, suite)
		}
		return nil
	}

	for _, p := range paths {
		dir, recursive, err := walkRoot(p)
		if err != nil {
			return result, err
		}
		if !recursive {
			if err := load(dir); err != nil {
				return result, err
			}
			continue
		}
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			// Nothing a dependency installed publishes a suite, and walking an
			// install tree costs a stat per directory in it.
			if d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return load(path)
		})
		if err != nil {
			return result, err
		}
	}

	sort.SliceStable(result.found, func(i, j int) bool {
		return result.found[i].Dir < result.found[j].Dir
	})
	return result, nil
}

// runSuite reads the phpscript.yml the invocation itself is under: the nearest
// one at or above the working directory.
//
// It is looked for upwards because that is where an application root sits
// relative to the tests it holds. A repository whose fixtures live in tests/
// writes one file at its root, and every fixture below inherits it without the
// tree repeating it per folder.
func runSuite(base config.Config) (*tests.Suite, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, nil
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, tests.SuiteFile)); err == nil {
			// The run's own file sets the keys describing the run, so the
			// run-wide ones are not refused here.
			return tests.LoadSuite(dir, base, false)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// walkRoot splits a fixture path argument into the directory to walk and
// whether the walk is recursive, matching tests.FindFixtures.
func walkRoot(p string) (string, bool, error) {
	p = filepath.Clean(p)
	recursive := false
	if strings.HasSuffix(p, "/...") || strings.HasSuffix(p, string(filepath.Separator)+"...") || p == "..." {
		recursive = true
		p = strings.TrimSuffix(p, "...")
		p = strings.TrimSuffix(p, string(filepath.Separator))
		if p == "" {
			p = "."
		}
	}

	info, err := os.Stat(p)
	if err != nil {
		return "", false, fmt.Errorf("stat %s: %w", p, err)
	}
	if !info.IsDir() {
		return filepath.Dir(p), false, nil
	}
	return p, recursive, nil
}

// applySuites points every fixture at the suite governing it: the connections
// its folder configured, and the prelude and application root it named.
//
// --include still wins over what a suite asked for, and speaks from the
// invocation root rather than from a suite: it is what an operator typed about
// this run, and the file it names sits where the command was invoked.
func applySuites(fixtures []*tests.Fixture, found suites, opts Options) {
	for _, fx := range fixtures {
		suite := found.resolve(fx.Path)
		fx.SetDatabase(suite.Provider())

		switch {
		case opts.Include != "":
			fx.SetAppRoot(".", opts.Include)
		case suite != nil && suite.Include() != "":
			fx.SetAppRoot(suite.Dir, suite.Include())
		}
	}
}

// hookOutput answers where a hook's own output goes.
//
// Only -v reports it. A hook writes whatever its migrations and its seed print,
// and without -v the run answers with a folder table that a schema's log lines
// would sit in the middle of. A failure is reported either way, by the error
// the run ends with.
func hookOutput(opts Options) io.Writer {
	if opts.Verbose && !opts.JSON && !opts.coverReport() {
		return os.Stdout
	}
	return io.Discard
}

// absDir answers the absolute spelling of a directory, or the given one when
// the working directory cannot be read. Two spellings of one directory must
// resolve to one suite, or its setup hook would run twice.
func absDir(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// sameDir reports whether two spellings name one directory.
func sameDir(a, b string) bool { return absDir(a) == absDir(b) }
