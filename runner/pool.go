package runner

import (
	"context"
	"io"
	goruntime "runtime"
	"sync"
)

// DefaultQueue is how many runs wait for a worker runtime: deep enough that a
// burst does not block the caller that produced it. Shallow enough that a queue
// this long means the workers are falling behind. An operator needs to learn
// that; nothing should absorb it.
const DefaultQueue = 1024

// Pool runs PHP on a fixed set of forks: the forks bound the parallelism. A
// run that finds every fork busy waits its turn and starts no extra runtime.
//
// A run executes on the goroutine that submits it, borrowing one of the
// pre-built runtimes. A submitted run parks and wakes no worker goroutine, which
// is half the context switches of a worker per request; the request goroutine
// net/http already has runs the handler.
type Pool struct {
	free    chan *poolWorker
	slots   chan struct{}
	workers int
	queue   int

	closing sync.RWMutex
	closed  bool

	stop     chan struct{}
	stopOnce sync.Once
}

// poolWorker names one fork and where its runs write.
type poolWorker struct {
	rt  *Runtime
	out func() io.Writer
}

// NewPool forks rt into workers runtimes, each borrowed by one run at a time.
//
// Zero workers is GOMAXPROCS, the parallelism for work that is all CPU. That
// fits a PHP handler doing no IO; a host whose handlers wait on a database
// wants more. Zero queue is DefaultQueue.
//
// The pool calls output once per worker, for where that worker's runtime
// writes until a run pushes a writer. A host wanting a run's output
// somewhere else pushes one inside the run; see Runtime.PushOutput.
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
		free:    make(chan *poolWorker, workers),
		slots:   make(chan struct{}, workers+queue),
		workers: workers,
		queue:   queue,
		stop:    make(chan struct{}),
	}
	for range workers {
		pool.free <- &poolWorker{rt: rt.Fork(output()), out: output}
	}
	return pool
}

// Workers answers how many runs the pool executes at once.
func (p *Pool) Workers() int { return p.workers }

// Queue answers how many runs it holds waiting.
func (p *Pool) Queue() int { return p.queue }

// Submit runs job on a borrowed runtime and waits for it. It reports whether
// the job ran.
//
// It answers false when ctx ends first, and on a closed pool. For a served
// request the first is the client leaving: the pool drops a waiting run. A
// started run continues to notice the disconnect through connection_aborted,
// because stopping it halfway is the handler's call.
func (p *Pool) Submit(ctx context.Context, job Job) bool {
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}
	// An ended context drops the run before a ready slot can win the select
	// below: two ready cases answer at random, and the caller asked to stop.
	if ended(done) {
		return false
	}

	// The slot bounds waiting runs the way the queue did: workers run and queue
	// more wait. A fuller pool blocks the caller. The fast path takes a
	// ready slot without a select, which is the path an idle pool answers on.
	select {
	case p.slots <- struct{}{}:
	default:
		select {
		case p.slots <- struct{}{}:
		case <-done:
			return false
		case <-p.stop:
			return false
		}
	}
	defer func() { <-p.slots }()

	p.closing.RLock()
	if p.closed {
		p.closing.RUnlock()
		return false
	}
	p.closing.RUnlock()

	// The ready worker takes the same fast path: a pool below its worker count
	// has a waiting worker for every submit.
	var worker *poolWorker
	select {
	case worker = <-p.free:
	default:
		select {
		case worker = <-p.free:
		case <-done:
			return false
		case <-p.stop:
			return false
		}
	}

	job.Run(worker.rt)
	// Reset after the run and not before and the next run starts on a runtime
	// holding nothing and this run drops the values it built now.
	worker.rt.resetExecution(worker.out(), nil)
	p.free <- worker
	return true
}

// Close stops the pool accepting runs; a borrowed run finishes on its caller.
func (p *Pool) Close() {
	p.stopOnce.Do(func() {
		p.closing.Lock()
		p.closed = true
		close(p.stop)
		p.closing.Unlock()
	})
}
