package runner

// Job is one unit of work a worker runs on its own runtime.
//
// An interface and no func and a caller whose receiver already lives for the
// length of the run submits that receiver and allocates no heap cell: a pointer
// boxes into an interface without a heap cell, where a closure capturing the
// same values is one per submit. stdlib/ps/http's per-request tracker is the
// shape this exists for.
type Job interface {
	Run(*Runtime)
}

// JobFunc adapts a function to Job, for a caller with nothing to hang the work
// on. The closure it wraps costs what a closure costs.
type JobFunc func(*Runtime)

// Run calls f.
func (f JobFunc) Run(rt *Runtime) { f(rt) }
