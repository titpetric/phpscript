package test

import (
	"testing"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/tests"
)

// TestOptionsFromConfig covers the layering a `test:` block gets: a value the
// command line named stands, and a key the block set reaches an option the
// flags left alone. given() is what tells the two apart.
func TestOptionsFromConfig(t *testing.T) {
	t.Parallel()
	opts := Options{Parallel: 8, typed: func(name string) bool { return name == "parallel" }}
	opts.fromConfig(config.Test{Parallel: 2, Cache: CacheOff})

	if opts.Parallel != 8 {
		t.Errorf("Parallel = %d, want the flag to win", opts.Parallel)
	}
	if opts.Cache != CacheOff {
		t.Errorf("Cache = %q, want the configured value where no flag named one", opts.Cache)
	}
}

// TestCacheModeNames covers the two scopes a run takes, and the refusal that
// names what it got: a mode nothing spells is a command line to correct.
func TestCacheModeNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{CacheOff, CacheWorker, ""} {
		if _, err := cacheMode(name); err != nil {
			t.Errorf("cacheMode(%q) = %v, want it accepted", name, err)
		}
	}
	if _, err := cacheMode("process"); err == nil {
		t.Error("cacheMode accepted a mode nothing spells")
	}
}

// TestOptionsRunners covers which runners a run answers with: all three by
// default, and the two in-process ones under --skip-php, which is the flag a
// machine with no php binary sets.
func TestOptionsRunners(t *testing.T) {
	t.Parallel()
	var none Options
	if got := none.runners(); len(got) != len(tests.Runners) {
		t.Errorf("runners() = %v, want every runner", got)
	}
	skipped := Options{SkipPHP: true}.runners()
	if len(skipped) != len(tests.Runners)-1 {
		t.Fatalf("runners() under --skip-php = %v, want one fewer", skipped)
	}
	for _, r := range skipped {
		if r == tests.RunnerPHP {
			t.Error("--skip-php kept the php runner")
		}
	}
}
