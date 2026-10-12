package runner

import (
	"context"
	"io"
	goruntime "runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestPool covers the two numbers a pool is built from, which are its contract:
// the worker count is the parallelism and the queue depth is the backpressure,
// so a caller can read both back off the pool it was handed.
//
// Zero workers is GOMAXPROCS, the parallelism the machine has for work that is
// all CPU, and zero queue is DefaultQueue.
func TestPool(t *testing.T) {
	rt := New(io.Discard, Options{})

	pool := NewPool(rt, 2, 8, nil)
	defer pool.Close()
	if got := pool.Workers(); got != 2 {
		t.Errorf("Workers() = %d, want 2", got)
	}
	if got := pool.Queue(); got != 8 {
		t.Errorf("Queue() = %d, want 8", got)
	}

	defaults := NewPool(rt, 0, 0, nil)
	defer defaults.Close()
	if got, want := defaults.Workers(), goruntime.GOMAXPROCS(0); got != want {
		t.Errorf("Workers() = %d on a zero worker count, want GOMAXPROCS %d", got, want)
	}
	if got := defaults.Queue(); got != DefaultQueue {
		t.Errorf("Queue() = %d on a zero queue, want DefaultQueue %d", got, DefaultQueue)
	}
}

// TestPool_Submit covers running work on a worker and waiting for it. The run
// is handed a forked runtime and not the one the pool was built from, which is
// what keeps two submits off one set of frames.
func TestPool_Submit(t *testing.T) {
	parent := New(io.Discard, Options{})
	pool := NewPool(parent, 1, 4, func() io.Writer { return io.Discard })
	defer pool.Close()

	var ran atomic.Int64
	var worker *Runtime
	if !pool.Submit(context.Background(), func(rt *Runtime) {
		ran.Add(1)
		worker = rt
	}) {
		t.Fatal("Submit() = false on an open pool")
	}
	if ran.Load() != 1 {
		t.Errorf("the run executed %d times, want once", ran.Load())
	}
	if worker == parent {
		t.Error("the run was handed the runtime the pool was forked from")
	}

	// A context that has already ended drops the run, and never starts it.
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	before := ran.Load()
	pool.Submit(ended, func(*Runtime) { ran.Add(1) })
	if ran.Load() != before {
		t.Error("a run submitted under an ended context was started")
	}
}

// TestPool_Close covers the pool refusing work once it is closed, and a second
// Close being harmless, because a host shutting down may reach it from more
// than one place.
func TestPool_Close(t *testing.T) {
	pool := NewPool(New(io.Discard, Options{}), 1, 4, nil)

	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within 5 seconds")
	}

	if pool.Submit(context.Background(), func(*Runtime) {}) {
		t.Error("Submit() = true on a closed pool")
	}
	pool.Close()
}
