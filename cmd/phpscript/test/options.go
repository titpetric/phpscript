package test

import (
	"fmt"
	"time"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/tests"
)

// Options holds CLI flag options for the test command. Include, Verbose, Cover
// and CoverFile are the shared flags, copied off internal/flags before the run
// so the report writers keep taking one struct.
type Options struct {
	Include   string
	JSON      bool
	Matrix    bool
	Verbose   bool
	Parallel  int
	Count     int
	Time      time.Duration
	Profile   bool
	Output    string
	Cover     string
	CoverFile string
	Split     bool
	SkipPHP   bool
	Cache     string

	// Config holds the starting state of the run: the embedded defaults, with
	// the file -f named read over them. discoverSuites reads a phpscript.yml from
	// the fixture tree over this one in turn.
	Config config.Config

	// typed reports whether the command line names a flag, choosing the flag.
	// A configuration-file value fills the rest. It stays nil for an Options a
	// test assembles, where none exists.
	typed func(string) bool
}

// fromConfig fills options the command line left unset, a flag beating the file.
func (o *Options) fromConfig(test config.Test) {
	if !o.given("parallel") && test.Parallel > 0 {
		o.Parallel = test.Parallel
	}
	if !o.given("cache") && test.Cache != "" {
		o.Cache = test.Cache
	}
	if !o.given("skip-php") && test.SkipPHP {
		o.SkipPHP = true
	}
}

// given reports whether the command line names the flag.
func (o Options) given(name string) bool {
	return o.typed != nil && o.typed(name)
}

// coverReport reports whether the cover mode owns stdout, leaving the tables off.
func (o Options) coverReport() bool {
	return o.Cover == CoverFunc || o.Cover == CoverFile
}

// runners answers the matrix columns, dropping php's column under --skip-php.
func (o Options) runners() []tests.Runner {
	if !o.SkipPHP {
		return tests.Runners
	}
	runners := make([]tests.Runner, 0, len(tests.Runners))
	for _, r := range tests.Runners {
		if r != tests.RunnerPHP {
			runners = append(runners, r)
		}
	}
	return runners
}

// The --cache modes, which set the travel of a parsed include and compiled
// expression.
//
// There is no "shared" mode: a worker loop already does that. Without
// --parallel there is a single worker, whose single cache set serves the
// whole run. Sharing them across workers on top of that only adds contention
// for a hit the worker's own cache already has.
const (
	// CacheOff builds caches per fixture run and drops the runtime
	// afterwards. A parse from one run stays invisible to the next. A run pays
	// the parsing of its includes, and drops the result at the end. No state
	// survives for the suite's length.
	CacheOff = "off"
	// CacheWorker keeps one set per worker, reused by the fixtures that
	// worker runs serially. A worker's holdings scale with --parallel, not
	// with the fixture count.
	CacheWorker = "worker"
)

// cacheMode reads the --cache flag.
func cacheMode(value string) (string, error) {
	switch value {
	case "", CacheWorker:
		return CacheWorker, nil
	case CacheOff:
		return value, nil
	}
	return "", fmt.Errorf("--cache must be %s or %s, not %q", CacheWorker, CacheOff, value)
}
