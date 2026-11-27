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

// suites holds the phpscript.yml files one invocation runs, its own first.
type suites struct {
	// run is the suite governing the invocation itself, or nil. Its directory
	// is not necessarily below the working directory. suites holds it apart
	// from the discovered suites; resolve answers it for every fixture that
	// finds no closer file.
	run *tests.Suite

	// found are the files discovered below the argument paths, sorted by
	// directory so setup runs an outer suite before a nested one.
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

// resolve returns the nearest suite at or above path, else the run's own.
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

// excluded reports whether the suite governing path lists it as skipped.
func (s suites) excluded(path string) bool {
	return s.resolve(path).Excluded(path)
}

// excludedDir reports whether the suite governing dir's contents skips dir.
func (s suites) excludedDir(dir string) bool {
	return s.resolve(filepath.Join(dir, "fixture.phpt")).Excluded(dir)
}

// dropExcluded removes the fixtures a suite's exclude list names.
func dropExcluded(fixtures []*tests.Fixture, found suites) []*tests.Fixture {
	kept := make([]*tests.Fixture, 0, len(fixtures))
	for _, fx := range fixtures {
		if found.excluded(fx.Path) {
			continue
		}
		kept = append(kept, fx)
	}
	return kept
}

// discoverSuites loads the run's suite plus every phpscript.yml below the walk's paths.
func discoverSuites(base config.Config, paths []string) (suites, error) {
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
			// An installed dependency publishes no suite, and walking an
			// install tree costs a stat per directory in it.
			if d.Name() == "vendor" {
				return filepath.SkipDir
			}
			if result.excludedDir(path) {
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

// runSuite reads the nearest phpscript.yml in the working directory or a parent.
func runSuite(base config.Config) (*tests.Suite, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, nil
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, tests.SuiteFile)); err == nil {
			// The run's own file sets the keys describing the run and the
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

// walkRoot splits a fixture path into its directory and recursive flag.
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

// applySuites points every fixture at its governing suite; --include overrides the app root.
func applySuites(fixtures []*tests.Fixture, found suites, opts Options) {
	for _, fx := range fixtures {
		suite := found.resolve(fx.Path)
		fx.SetDatabase(suite.Provider())
		fx.SetMail(suite.Mail())

		switch {
		case opts.Include != "":
			fx.SetAppRoot(".", opts.Include)
		case suite != nil && suite.Include() != "":
			fx.SetAppRoot(suite.Dir, suite.Include())
		}
	}
}

// hookOutput returns stdout under -v and discards a hook's output otherwise.
func hookOutput(opts Options) io.Writer {
	if opts.Verbose && !opts.JSON && !opts.coverReport() {
		return os.Stdout
	}
	return io.Discard
}

// absDir returns dir's absolute spelling, or dir when filepath.Abs fails.
func absDir(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// sameDir reports whether two spellings name one directory.
func sameDir(a, b string) bool { return absDir(a) == absDir(b) }
