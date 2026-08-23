package test

import (
	"path/filepath"
	"time"

	"github.com/titpetric/phpscript/tests"
)

// fixtureGroup is the fixtures of one folder, in discovery order. Grouping is
// what lets the runner print a table per area instead of one long table, so
// the fixtures of a folder are read as a set.
type fixtureGroup struct {
	Dir      string // folder holding the fixtures, "." when they sit in the run directory
	Fixtures []*tests.Fixture
	Paths    []string // full display path, per fixture
	Labels   []string // basename, which is what the table shows
}

// groupTotals is one folder's contribution to the run.
type groupTotals struct {
	Dir      string
	Passed   int
	Failed   int
	Total    int
	Duration time.Duration
}

// groupFixtures buckets fixtures by the folder holding them, preserving the
// order discovery established both between folders and inside one. Bucketing
// is explicit rather than relying on the sort, because sorted paths do not
// keep a folder contiguous: a/b.phpt, a/m/x.phpt and a/z.phpt interleave.
func groupFixtures(fixtures []*tests.Fixture, displayPaths []string) []fixtureGroup {
	index := map[string]int{}
	var groups []fixtureGroup

	for i, fx := range fixtures {
		display := displayPaths[i]
		dir := filepath.ToSlash(filepath.Dir(display))
		at, ok := index[dir]
		if !ok {
			at = len(groups)
			index[dir] = at
			groups = append(groups, fixtureGroup{Dir: dir})
		}
		groups[at].Fixtures = append(groups[at].Fixtures, fx)
		groups[at].Paths = append(groups[at].Paths, display)
		groups[at].Labels = append(groups[at].Labels, filepath.Base(display))
	}

	return groups
}
