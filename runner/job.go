package runner

// Job is a unit of work a worker runs on a private runtime.
//
// The shape is an interface. A caller whose receiver lives for the run submits
// that receiver and allocates no heap cell. A pointer boxes into an interface
// and needs no heap cell. A closure capturing the same values allocates a cell
// per submit. stdlib/ps/http's per-request tracker is the shape this exists for.
type Job interface {
	Run(*Runtime)
}

// JobFunc adapts a function to Job, for a caller holding no receiver for the
// work. The closure it wraps costs as much as any closure.
type JobFunc func(*Runtime)

// Run calls f.
func (f JobFunc) Run(rt *Runtime) { f(rt) }
