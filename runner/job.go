package runner

// Job is one unit of work a worker runs on its own runtime.
//
// An interface and no func, so a caller whose receiver already lives for the
// length of the run submits that receiver and allocates nothing: a pointer
// boxes into an interface without a heap cell, where a closure capturing the
// same values is one per submit. stdlib/http's per-request tracker is the
// shape this exists for.
type Job interface {
	Run(*Runtime)
}

// JobFunc adapts a function to Job, for a caller with nothing to hang the work
// on. The closure it wraps costs what a closure costs.
type JobFunc func(*Runtime)

// Run calls f.
func (f JobFunc) Run(rt *Runtime) { f(rt) }

// DoneJob is a Job that lends the pool a channel to signal its completion
// through, so a job serving many runs carries one channel in place of the pool
// building one per submit. One buffered slot, which the worker sends into and
// the submitter drains, so the channel serves the next run.
//
// A lender drops a job it submitted and did not see finish.
// docs/allocation-performance.md argues that rule and why the pool keeps no
// free list of its own.
type DoneJob interface {
	Job
	Done() chan struct{}
}
