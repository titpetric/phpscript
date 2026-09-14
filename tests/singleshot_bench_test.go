package tests

import (
	"testing"
)

// benchFixture loads one embedded fixture by path for the single-shot
// benchmarks below.
func benchFixture(b *testing.B, path string) *Fixture {
	b.Helper()
	areas, err := embeddedFixtures()
	if err != nil {
		b.Fatal(err)
	}
	for _, area := range areas {
		for _, fx := range area.Fixtures {
			if fx.Path == path {
				return fx
			}
		}
	}
	b.Fatalf("fixture %s not found", path)
	return nil
}

// The single-shot benchmarks measure what one cold `phpscript test` run pays
// per fixture and engine: cache scope off gives every iteration fresh caches
// and a fresh runtime, so an iteration is a first run, compile included. The
// parsed AST stays cached on the fixture, as it does for both engines in the
// harness.
func benchSingleShot(b *testing.B, path string, runner Runner) {
	fx := benchFixture(b, path)
	fx.SetCacheScope("off")
	b.ReportAllocs()
	for b.Loop() {
		res := RunFixtureOn(b.Context(), fx, runner)
		if !res.Passed {
			b.Fatalf("fixture failed: %s", res.FailureReason)
		}
	}
}

func BenchmarkSingleShotFlatstack(b *testing.B) {
	benchSingleShot(b, "fixtures/arrays/array_stack.phpt", RunnerFlatstack)
}

func BenchmarkSingleShotRuntime(b *testing.B) {
	benchSingleShot(b, "fixtures/arrays/array_stack.phpt", RunnerRuntime)
}
