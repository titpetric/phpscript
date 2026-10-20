package runner

import (
	"context"
	"io"
	goruntime "runtime"
	"sync"
)

// DefaultQueue is how many runs a pool holds waiting for a worker: deep enough
// that a burst does not block the caller that produced it, and shallow enough
// that a queue this long means the workers are falling behind. An operator
// needs to learn that, and no queue should absorb it.
const DefaultQueue = 1024

// Pool runs PHP on a fixed set of forks, fed from one queue: the workers are
// the parallelism and the queue is the backpressure, so a run that finds every
// worker busy waits its turn and starts no runtime of its own.
//
// Nothing here is a sync.Pool. The worker count is the contract, and sync.Pool
// drops entries on a collection.
type Pool struct {
	runs    chan poolRun
	workers int
	queue   int

	// A Submit about to send takes closing for reading, and the Close that
	// shuts the channel takes it for writing. Without it the select in Submit
	// has two ready cases on a closed pool - the send and the stop - and picks
	// between them at random, so one caller in two panics on a send to a
	// closed channel and the other hears that the pool has gone.
	closing sync.RWMutex
	closed  bool

	stop     chan struct{}
	stopOnce sync.Once
	done     sync.WaitGroup
}

// poolRun is one unit of work and the channel its submitter waits on.
type poolRun struct {
	job  Job
	done chan struct{}
}

// NewPool forks rt into workers runtimes, each reading from a queue of depth
// queue, and starts them.
//
// Zero workers is GOMAXPROCS, which is the parallelism the machine has for work
// that is all CPU, and is what a PHP handler doing no IO is; a host whose
// handlers wait on a database wants more. Zero queue is DefaultQueue.
//
// The pool calls output once per worker, for where that worker's runtime
// writes while no writer sits over it. A host wanting a run's output to reach
// somewhere of its own pushes a writer inside the run; see Runtime.PushOutput.
func NewPool(rt *Runtime, workers, queue int, output func() io.Writer) *Pool {
	if workers <= 0 {
		workers = goruntime.GOMAXPROCS(0)
	}
	if queue <= 0 {
		queue = DefaultQueue
	}
	if output == nil {
		output = func() io.Writer { return io.Discard }
	}

	pool := &Pool{
		runs:    make(chan poolRun, queue),
		workers: workers,
		queue:   queue,
		stop:    make(chan struct{}),
	}

	pool.done.Add(workers)
	for range workers {
		worker := rt.Fork(output())
		go func() {
			defer pool.done.Done()
			for run := range pool.runs {
				run.job.Run(worker)
				// Reset after the run and not before, so the next run starts
				// on a runtime holding nothing and this run drops the values
				// it built now, not at the next request.
				worker.resetExecution(output(), nil)
				// A send and no close, so a lent channel serves the job's
				// next run. The slot holds one value, so a caller that
				// stopped waiting leaves this worker free and parks nothing.
				run.done <- struct{}{}
			}
		}()
	}
	return pool
}

// Workers answers how many runs the pool executes at once.
func (p *Pool) Workers() int { return p.workers }

// Queue answers how many runs it holds waiting.
func (p *Pool) Queue() int { return p.queue }

// Submit runs fn on a worker and waits for it, and reports whether it ran.
//
// It answers false when ctx ends first, and on a closed pool. For a served
// request the first is the client leaving: the pool drops a queued run, and a
// started one runs on to notice the disconnect through connection_aborted,
// because stopping it halfway is the handler's call.
//
// Why the per-submit done channel is not recycled is
// docs/allocation-performance.md, "The per-submit done channel".
func (p *Pool) Submit(ctx context.Context, job Job) bool {
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}
	// The lender's channel in place of one per submit, and the make only
	// where no job carries one.
	var finished chan struct{}
	if lender, ok := job.(DoneJob); ok {
		finished = lender.Done()
	} else {
		finished = make(chan struct{}, 1)
	}
	run := poolRun{job: job, done: finished}

	// The read lock spans the send alone. A worker is still draining while
	// Close waits for the write lock, so a send that found the queue full
	// completes and never holds the shutdown.
	p.closing.RLock()
	if p.closed {
		p.closing.RUnlock()
		return false
	}
	select {
	case p.runs <- run:
		p.closing.RUnlock()
	case <-done:
		p.closing.RUnlock()
		return false
	case <-p.stop:
		p.closing.RUnlock()
		return false
	}

	select {
	case <-run.done:
		return true
	case <-done:
		// Queued or running, the run belongs to the worker now: the worker
		// closes the run channel, and waiting here any longer would hold the
		// caller for a client that has gone.
		return false
	}
}

// Close stops the workers once they finish the runs already queued.
func (p *Pool) Close() {
	p.stopOnce.Do(func() {
		p.closing.Lock()
		p.closed = true
		close(p.stop)
		close(p.runs)
		p.closing.Unlock()
	})
	p.done.Wait()
}
