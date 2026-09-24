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
