package test

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/titpetric/phpscript/list"
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

// mapFixtures runs fn over a group with bounded concurrency and returns values
// in fixture discovery order, keeping terminal, Markdown, and JSON output stable.
func mapFixtures[T any](fixtures []*tests.Fixture, parallel int, fn func(int, *tests.Fixture) T) []T {
	results := make([]T, len(fixtures))
	if parallel <= 1 || len(fixtures) <= 1 {
		for i, fx := range fixtures {
			results[i] = fn(i, fx)
		}
		return results
	}

	for start := 0; start < len(fixtures); {
		if fixtures[start].Serial {
			results[start] = fn(start, fixtures[start])
			start++
			continue
		}
		end := start + 1
		for end < len(fixtures) && !fixtures[end].Serial {
			end++
		}
		mapFixtureBatch(fixtures, results, start, end, parallel, fn)
		start = end
	}
	return results
}

func mapFixtureBatch[T any](fixtures []*tests.Fixture, results []T, start, end, parallel int, fn func(int, *tests.Fixture) T) {
	jobs := make(chan int)
	var workers sync.WaitGroup
	workers.Add(min(parallel, end-start))
	for range min(parallel, end-start) {
		go func() {
			defer workers.Done()
			for i := range jobs {
				results[i] = fn(i, fixtures[i])
			}
		}()
	}
	for i := start; i < end; i++ {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
}

// groupFixtures buckets fixtures by the folder holding them, preserving the
// order discovery established both between folders and inside one, which is
// what list.GroupByDir does for any path list.
func groupFixtures(fixtures []*tests.Fixture, displayPaths []string) []fixtureGroup {
	buckets := list.GroupByDir(displayPaths)
	groups := make([]fixtureGroup, 0, len(buckets))

	for _, bucket := range buckets {
		group := fixtureGroup{Dir: bucket.Dir}
		for _, i := range bucket.Indexes {
			group.Fixtures = append(group.Fixtures, fixtures[i])
			group.Paths = append(group.Paths, displayPaths[i])
			group.Labels = append(group.Labels, filepath.Base(displayPaths[i]))
		}
		groups = append(groups, group)
	}

	return groups
}
