package runner

import (
	"io"
	"testing"
)

// TestJobFunc_Run covers the adapter: a function submitted through it reaches
// the worker's runtime as the Job the pool runs, so a caller with nothing to
// hang the work on keeps the plain shape. Pool.Submit holds the paths that
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

// TestJobFuncSatisfiesJobAlone covers which of the three a bare function
// answers: Job, and never DoneJob. A JobFunc lends no channel, so Submit
// builds one, and a type assertion deciding that has to read false here.
func TestJobFuncSatisfiesJobAlone(t *testing.T) {
	t.Parallel()
	var job Job = JobFunc(func(*Runtime) {})

	if _, ok := job.(DoneJob); ok {
		t.Error("a JobFunc answers DoneJob, so Submit would read a channel it does not have")
	}
}
