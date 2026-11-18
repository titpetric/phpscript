package tests

import (
	"context"
	"testing"
)

// TestWithWorker covers the cache lookup of a fixture run. Go has no
// goroutine-local storage; the worker id rides the run's context, already
// threaded from the command down.
//
// A context nothing marked answers worker zero. That worker is like the
// others. A single-threaded run lands there; a Go test calling RunFixture
// directly lands there too.
func TestWithWorker(t *testing.T) {
	plain := context.Background()
	if got := currentWorker(plain); got == nil {
		t.Fatal("an unmarked context answered no worker")
	}

	marked := WithWorker(plain, 3)
	if marked == plain {
		t.Fatal("WithWorker answered the context it was given")
	}
	if got, ok := marked.Value(workerKey).(int); !ok || got != 3 {
		t.Errorf("the marked context carries worker %v, want 3", marked.Value(workerKey))
	}

	// The same id answers the same worker. Two runs on one goroutine share its
	// caches and build a single set.
	if currentWorker(marked) != currentWorker(WithWorker(plain, 3)) {
		t.Error("two contexts naming worker 3 answered different workers")
	}
	if currentWorker(marked) == currentWorker(WithWorker(plain, 4)) {
		t.Error("worker 3 and worker 4 answered the same caches")
	}
}
