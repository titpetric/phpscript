package test

import (
	"context"
	"testing"
	"time"

	"github.com/titpetric/phpscript/tests"
)

func loopFixture(t *testing.T) *tests.Fixture {
	t.Helper()
	src := "name: loop\ndescription: loop harness fixture\n---\n<?php echo \"ok\";\n---\nok"
	fx, err := tests.ParseFixture([]byte(src), "loop.phpt")
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func TestRunFixtureLoopCount(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{Count: 5})
	if fr.Runs != 5 {
		t.Fatalf("Runs = %d, want 5", fr.Runs)
	}
	if !fr.Result.Passed {
		t.Fatalf("fixture failed: %s", fr.Result.FailureReason)
	}
}

func TestRunFixtureLoopTime(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{Time: 50 * time.Millisecond})
	if fr.Runs < 2 {
		t.Fatalf("Runs = %d, want at least 2 within 50ms", fr.Runs)
	}
	if fr.Total < 50*time.Millisecond {
		t.Fatalf("Total = %v, want >= 50ms", fr.Total)
	}
}

func TestRunFixtureLoopSingleByDefault(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{})
	if fr.Runs != 1 {
		t.Fatalf("Runs = %d, want 1", fr.Runs)
	}
}

func TestRunFixtureLoopProfile(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{Count: 3, Profile: true})
	if fr.AllocsPerOp == 0 || fr.BytesPerOp == 0 {
		t.Fatalf("profile counters empty: allocs=%d bytes=%d", fr.AllocsPerOp, fr.BytesPerOp)
	}
}

func TestRunFixtureSamplesCountAndTime(t *testing.T) {
	const (
		count     = 3
		benchTime = 10 * time.Millisecond
	)
	runs := runFixtureSamples(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{
		Count: count,
		Time:  benchTime,
	})
	if len(runs) != count {
		t.Fatalf("sample count = %d, want %d", len(runs), count)
	}
	for i, run := range runs {
		if run.Total < benchTime {
			t.Errorf("sample %d duration = %v, want at least %v", i, run.Total, benchTime)
		}
		if run.Runs < 1 {
			t.Errorf("sample %d operation count = %d, want at least 1", i, run.Runs)
		}
	}
}

func TestPercentileNs(t *testing.T) {
	samples := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got, want := percentileNs(samples, 50), int64(50); got != want {
		t.Fatalf("p50 = %d, want %d", got, want)
	}
	if got, want := percentileNs(samples, 95), int64(100); got != want {
		t.Fatalf("p95 = %d, want %d", got, want)
	}
	if got := percentileNs(nil, 50); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
}
