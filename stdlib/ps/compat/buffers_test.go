package compat

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestBuffersNest covers the stack discipline a template engine renders
// through: ob_start pushes a level, a flush writes into the level below it,
// and a clean drops its text. Writing a flushed level to the real output
// instead would make an outer buffer miss everything an inner one rendered.
func TestBuffersNest(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	b := newBuffers(rt)

	b.push()
	rt.Output().Write([]byte("outer-"))
	b.push()
	rt.Output().Write([]byte("inner"))

	if got := b.level(); got != 2 {
		t.Fatalf("level() = %d, want 2", got)
	}

	// The inner level flushes into the outer one and the real writer is
	// still empty and the outer buffer now holds both halves.
	if contents, ok := b.pop(true); !ok || contents != "inner" {
		t.Fatalf("pop(flush) = %q, %t", contents, ok)
	}
	if out.String() != "" {
		t.Errorf("the writer holds %q while a level is still open", out.String())
	}
	if contents, _ := b.contents(); contents != "outer-inner" {
		t.Errorf("the outer level holds %q, want both halves", contents)
	}

	if contents, ok := b.pop(true); !ok || contents != "outer-inner" {
		t.Fatalf("pop(flush) = %q, %t", contents, ok)
	}
	if got, want := out.String(), "outer-inner"; got != want {
		t.Errorf("the writer holds %q, want %q", got, want)
	}
	if got := b.level(); got != 0 {
		t.Errorf("level() = %d after both levels closed", got)
	}
}

// TestBuffersDiscardOnClean covers ob_end_clean: the level closes and pop drops
// its text and a template that rendered something a page did not want leaves
// no text in the output below it.
func TestBuffersDiscardOnClean(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	b := newBuffers(rt)

	rt.Output().Write([]byte("before;"))
	b.push()
	rt.Output().Write([]byte("captured;"))
	if contents, ok := b.pop(false); !ok || contents != "captured;" {
		t.Fatalf("pop(discard) = %q, %t", contents, ok)
	}
	rt.Output().Write([]byte("after"))

	if got, want := out.String(), "before;after"; got != want {
		t.Errorf("the writer holds %q, want %q", got, want)
	}
}

// TestBuffersReportAbsence covers the result PHP distinguishes with ===: with
// no level open, contents and pop report false and never an empty string and a
// script can tell an empty buffer from no buffer at all.
func TestBuffersReportAbsence(t *testing.T) {
	t.Parallel()
	b := newBuffers(runner.New(&strings.Builder{}, runner.Options{}))

	if contents, ok := b.contents(); ok || contents != "" {
		t.Errorf("contents() = %q, %t with no level open", contents, ok)
	}
	for _, flush := range []bool{false, true} {
		if contents, ok := b.pop(flush); ok || contents != "" {
			t.Errorf("pop(%t) = %q, %t with no level open", flush, contents, ok)
		}
	}
	if got := b.level(); got != 0 {
		t.Errorf("level() = %d on a fresh stack", got)
	}
}
