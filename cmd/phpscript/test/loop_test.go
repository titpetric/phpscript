package test

import (
	"bytes"
	"context"
	"strings"
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
	fr := runFixtureLoop(context.Background(), loopFixture(t), Options{Count: 5})
	if fr.Runs != 5 {
		t.Fatalf("Runs = %d, want 5", fr.Runs)
	}
	if !fr.Result.Passed {
		t.Fatalf("fixture failed: %s", fr.Result.FailureReason)
	}
}

func TestRunFixtureLoopTime(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), Options{Time: 50 * time.Millisecond})
	if fr.Runs < 2 {
		t.Fatalf("Runs = %d, want at least 2 within 50ms", fr.Runs)
	}
	if fr.Total < 50*time.Millisecond {
		t.Fatalf("Total = %v, want >= 50ms", fr.Total)
	}
}

func TestRunFixtureLoopSingleByDefault(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), Options{})
	if fr.Runs != 1 {
		t.Fatalf("Runs = %d, want 1", fr.Runs)
	}
}

func TestRunFixtureLoopProfile(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), Options{Count: 3, Profile: true})
	if fr.AllocsPerOp == 0 || fr.BytesPerOp == 0 {
		t.Fatalf("profile counters empty: allocs=%d bytes=%d", fr.AllocsPerOp, fr.BytesPerOp)
	}
}

func TestMarkdownTableColumns(t *testing.T) {
	fr := &fixtureRun{
		Result:      &tests.TestResult{Passed: true},
		DisplayPath: "a.phpt",
		Runs:        2,
		Total:       3 * time.Millisecond,
		AllocsPerOp: 7,
		BytesPerOp:  11,
	}
	cases := []struct {
		opts   Options
		header string
	}{
		{Options{}, "| Status | Filename | Duration |"},
		{Options{Count: 2}, "| Status | Filename | Duration | Count |"},
		{Options{Count: 2, Profile: true}, "| Status | Filename | Duration | Count | Allocations/op | Bytes/op |"},
		{Options{Profile: true}, "| Status | Filename | Duration | Allocations/op | Bytes/op |"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		writeMarkdownTable(&buf, []*fixtureRun{fr}, c.opts)
		first := strings.SplitN(buf.String(), "\n", 2)[0]
		if first != c.header {
			t.Errorf("opts %+v: header = %q, want %q", c.opts, first, c.header)
		}
	}
}
