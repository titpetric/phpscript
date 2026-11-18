package test

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/tests"
)

// fixtureGroup holds one folder's fixtures in discovery order.
type fixtureGroup struct {
	Dir      string // folder holding the fixtures, "." when they are in the run directory
	Fixtures []*tests.Fixture
	Paths    []string // full display path, per fixture
	Labels   []string // basename, the name the table shows
}

// groupTotals holds one folder's contribution to the run.
type groupTotals struct {
	Dir      string
	Passed   int
	Failed   int
	Total    int
	Duration time.Duration
	// Engines is the per-runner share of the time, in report order. Only a
	// matrix run fills the list. A plain run has a single engine, and Duration
	// already reports it.
	Engines []engineDuration
}

// engineDuration holds the time one runner spent on its fixture runs.
type engineDuration struct {
	Runner   tests.Runner
	Duration time.Duration
}

// mapFixtures runs fn with bounded concurrency and returns values in discovery order.
func mapFixtures[T any](fixtures []*tests.Fixture, parallel int, fn func(worker, i int, fx *tests.Fixture) T) []T {
	results := make([]T, len(fixtures))
	if parallel <= 1 || len(fixtures) <= 1 {
		for i, fx := range fixtures {
			results[i] = fn(0, i, fx)
		}
		return results
	}

	for start := 0; start < len(fixtures); {
		if fixtures[start].Serial {
			results[start] = fn(0, start, fixtures[start])
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

func mapFixtureBatch[T any](fixtures []*tests.Fixture, results []T, start, end, parallel int, fn func(worker, i int, fx *tests.Fixture) T) {
	// Buffered to the whole batch. The sender queues the fixtures in one go,
	// and the workers draw from the channel. An unbuffered channel makes the
	// sender wait for a worker to take each fixture, which serialises the
	// hand-off against the slowest fixture in flight.
	jobs := make(chan int, end-start)
	var workers sync.WaitGroup
	workers.Add(min(parallel, end-start))
	for id := range min(parallel, end-start) {
		// Each goroutine is a worker with a number. The --cache=worker setting
		// scopes a cache to the number. The loop is serial. Two fixtures never
		// touch a worker's cache at once.
		go func(id int) {
			defer workers.Done()
			for i := range jobs {
				results[i] = fn(id, i, fixtures[i])
			}
		}(id)
	}
	for i := start; i < end; i++ {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
}

// groupFixtures buckets fixtures by their folder, preserving discovery order.
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
