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

	// Config is what the run started with: the embedded defaults, with the
	// file -f named read over them. discoverSuites reads a phpscript.yml from
	// the fixture tree over this one in turn.
	Config config.Config

	// typed reports whether the command line carries a flag, and so decides
	// between it and the value a configuration file supplies. It stays nil
	// for an Options a test assembles, where no command line exists.
	typed func(string) bool
}

// fromConfig fills the run-wide options a test block may also carry, where the
// command line did not name one. The flag wins: a configuration describes a
// tree, and a flag is what an operator typed about this run of it.
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

// given reports whether the command line carries the named flag.
func (o Options) given(name string) bool {
	return o.typed != nil && o.typed(name)
}

// coverReport reports whether the cover mode owns stdout with a per-symbol
// report. The run leaves the fixture tables off stdout when it does.
func (o Options) coverReport() bool {
	return o.Cover == CoverFunc || o.Cover == CoverFile
}

// runners answers the matrix columns this invocation covers: every backend,
// minus php when --skip-php dropped the external binary from the run. The
// column leaves the table entirely, with no SKIP per row: a
// skipped column says the machine has no php, this flag says do not ask.
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

// The --cache modes, which say how far a parsed include and a compiled
// expression travel.
//
// There is no "shared" mode because a worker loop already is one: run without
// --parallel there is a single worker and a single set of caches serves the
// whole run. Sharing them across workers on top of that only adds contention
// for a hit the worker's own cache already has.
const (
	// CacheOff builds caches per fixture run and drops the runtime
	// after it. A parse from one run stays invisible to the next, and a run
	// pays the parsing its own includes cost - and drops what it held when it
	// ends, with no state kept for the length of the suite.
	CacheOff = "off"
	// CacheWorker keeps one set per worker, reused by the fixtures that
	// worker runs serially. What a worker holds scales with --parallel and
	// not with the number of fixtures.
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
