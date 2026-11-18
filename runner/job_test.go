package runner

import (
	"io"
	"testing"
)

// TestJobFunc_Run covers the adapter: a function submitted through the adapter
// reaches the worker's runtime as the Job the pool runs; a caller with nothing
// to hang the work on keeps the plain shape. Pool.Submit holds the paths that
// matter for the interface itself.
func TestJobFunc_Run(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	var got *Runtime
	var job Job = JobFunc(func(r *Runtime) { got = r })
	job.Run(rt)

	if got != rt {
		t.Errorf("the job ran on %p, want the runtime it is handed", got)
	}
}
