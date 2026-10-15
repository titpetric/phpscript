package core_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/core"
)

func TestSharedMemoryBindingAcrossRequests(t *testing.T) {
	t.Parallel()
	shm := core.NewSharedMemory()
	runRequest := func(src string) string {
		t.Helper()

		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		buf := new(bytes.Buffer)
		rt := flatstack.New(buf, flatstack.Options{})
		rt.SetContext(core.SharedMemoryContext(context.Background(), shm))
		stdlib.Register(rt)

		if err := rt.Run(prog); err != nil {
			t.Fatalf("Run error: %v", err)
		}
		return buf.String()
	}

	first := runRequest(`<?php
		$shm = new SharedMemory;
		$shm->incr("requests");
		echo $shm->count("requests");
	?>`)
	if first != "1" {
		t.Fatalf("first request got %q, want %q", first, "1")
	}

	second := runRequest(`<?php
		$shm = new SharedMemory;
		$shm->incr("requests");
		echo $shm->count("requests");
	?>`)
	if second != "2" {
		t.Errorf("second request got %q, want %q", second, "2")
	}
}

// TestSharedMemoryInfo covers what phpinfo() reports about a store a host
// bound into the runtime. A run with none bound has nothing to report, because
// `new SharedMemory` answers a fresh store per call there.
func TestSharedMemoryInfo(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	core.RegisterSharedMemory(rt)

	if err := rt.PHPInfo(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "SharedMemory") {
		t.Fatalf("an unbound store printed a section:\n%s", out.String())
	}

	store := core.NewSharedMemory()
	store.Set(context.Background(), "alpha", strings.Repeat("x", 2048))
	store.Incr(context.Background(), "hits")
	rt.SetContext(core.SharedMemoryContext(context.Background(), store))

	out.Reset()
	if err := rt.PHPInfo(); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	for _, want := range []string{"\nSharedMemory\n", "Entries => 1", "Counters => 1", "Size =>", " MiB"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report is missing %q:\n%s", want, report)
		}
	}

	entries, counters, bytes := store.Usage()
	if entries != 1 || counters != 1 || bytes < 2048 {
		t.Fatalf("Usage() = %d, %d, %d; want 1, 1, at least 2048", entries, counters, bytes)
	}
}

// TestSharedMemory_Get covers reading a value back, and what a key nothing set
// reads as. The binding has one return value and no found flag, so a miss is
// the empty string; Has is the separate question.
func TestSharedMemory_Get(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := core.NewSharedMemory()

	if got := store.Get(ctx, "absent"); got != "" {
		t.Errorf("a key nothing set reads as %q, want the empty string", got)
	}

	store.Set(ctx, "greeting", "hello")
	if got := store.Get(ctx, "greeting"); got != "hello" {
		t.Errorf("Get() = %q, want %q", got, "hello")
	}

	store.Set(ctx, "greeting", "second")
	if got := store.Get(ctx, "greeting"); got != "second" {
		t.Errorf("a second Set left %q, want %q", got, "second")
	}
}

// TestSharedMemory_Count covers reading a counter without moving it. Count
// answers a decimal string where Incr answers the int, so a template prints the
// figure with no cast, and a counter nothing touched reads as "0".
func TestSharedMemory_Count(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := core.NewSharedMemory()

	if got := store.Count(ctx, "hits"); got != "0" {
		t.Errorf("an untouched counter reads as %q, want %q", got, "0")
	}

	store.Incr(ctx, "hits")
	store.Incr(ctx, "hits")
	if got := store.Count(ctx, "hits"); got != "2" {
		t.Errorf("Count() = %q after two increments, want %q", got, "2")
	}
	if got := store.Count(ctx, "hits"); got != "2" {
		t.Errorf("a second Count() = %q, want %q; reading moves the counter", got, "2")
	}
}

// TestSharedMemory_Delete covers removing a name and reporting whether it is
// present. One name holds a value or a counter, so the call clears it from the
// values, then from the counters.
func TestSharedMemory_Delete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := core.NewSharedMemory()
	store.Set(ctx, "value", "x")
	store.Incr(ctx, "counter")

	for _, key := range []string{"value", "counter"} {
		if !store.Delete(ctx, key) {
			t.Errorf("Delete(%q) = false on a key that was set", key)
		}
		if store.Has(ctx, key) {
			t.Errorf("%q is still present after Delete", key)
		}
	}

	if store.Delete(ctx, "absent") {
		t.Error("Delete() = true for a key nothing set")
	}

	entries, counters, _ := store.Usage()
	if entries != 0 || counters != 0 {
		t.Errorf("Usage() reports %d entries and %d counters, want 0 and 0", entries, counters)
	}
}

// TestSharedMemory_Has covers the one question Get cannot answer: a key set to
// the empty string reads the same as a key nothing set, so presence is its own
// call. A counter counts as present.
func TestSharedMemory_Has(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := core.NewSharedMemory()

	if store.Has(ctx, "blank") {
		t.Error("Has() = true before anything was set")
	}

	store.Set(ctx, "blank", "")
	if !store.Has(ctx, "blank") {
		t.Error("Has() = false for a key set to the empty string")
	}

	store.Incr(ctx, "counter")
	if !store.Has(ctx, "counter") {
		t.Error("Has() = false for a counter")
	}
}

// TestSharedMemory_Clear covers emptying the store through the same pointer a
// host bound into its context, so every request holding the store sees the
// reset.
func TestSharedMemory_Clear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := core.NewSharedMemory()
	store.Set(ctx, "value", "x")
	store.Incr(ctx, "counter")

	store.Clear(ctx)

	entries, counters, bytes := store.Usage()
	if entries != 0 || counters != 0 || bytes != 0 {
		t.Errorf("Usage() = %d, %d, %d after Clear, want 0, 0, 0", entries, counters, bytes)
	}

	store.Incr(ctx, "counter")
	if got := store.Count(ctx, "counter"); got != "1" {
		t.Errorf("the counter resumed at %q, want %q", got, "1")
	}
}
