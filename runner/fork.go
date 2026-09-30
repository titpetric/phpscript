package runner

import (
	"context"
	"io"
	goruntime "runtime"
	"sync"

	"github.com/titpetric/phpscript/model"
)

// This file is how PHP is reached from more than one goroutine.
//
// A Runtime is one execution: its frames, its compiled-expression memo, its Go
// method cache, its output and its statics are all unguarded, because a script
// is one program on one goroutine. Sharing one across goroutines is not a race
// to reason about, it is a concurrent map write.
//
// A fork is a second Runtime with the same symbols and the same caches and none
// of the execution state. Where a VM with threads would keep a stack per thread
// and index it by thread id, this keeps a whole runtime per worker and indexes
// it by the handle the worker holds: a Go program cannot read a goroutine id
// without parsing a stack trace, which costs around four microseconds, and the
// frames are only one of the six things that would have to be split anyway.
//
// What a fork does not carry is anything a program wrote: globals, statics,
// constants a script defined, output. It carries what a host installed and what
// the tree declared. That is sound here because a phpscript function is a
// function of its arguments - there is no `global` statement (docs/design.md)
// and no request state unless a host seeds it - so two forks running the same
// declaration answer the same thing.

// Fork returns a runtime that can run the same symbols as this one, writing to
// w, and shares its parse and bytecode caches.
//
// What is copied is the symbol tables a host registered and the declarations
// the tree hoisted. What is not is every part of an execution: frames, globals,
// statics, the constants a script defined, the superglobals, the output stack,
// the included list, the shutdown callbacks and the memory accounting. A fork
// starts as if nothing had run on it, which is what makes it safe to run on a
// goroutine of its own.
//
// Forking is not cheap enough for a request. It installs the whole standard
// library, which is around three hundred registrations. A Pool is how a host
// pays it once per worker.
func (rt *Runtime) Fork(w io.Writer) *Runtime {
	child := New(w, rt.opts)
	child.flat = rt.flat
	child.includeCache = rt.includeCache
	child.exprCache = rt.exprCache
	child.include = rt.include
	child.includePath = rt.includePath
	child.observers = rt.observers
	child.errorHandler = rt.errorHandler
	child.Env = rt.Env
	child.SetContext(rt.host)

	// The bindings, installed rather than copied.
	//
	// A binding is almost always a closure over the runtime it was registered
	// on - header() reaches for that runtime's request, ignore_user_abort sets
	// that runtime's flag - so a copied entry would read and write the parent
	// from inside the child. The host says how to install them and the child
	// gets its own.
	//
	// Entries are copied first all the same, for a host that registered a
	// stateless function by hand and set no preparer; the install overwrites
	// every name it covers.
	for name, entry := range rt.funcs {
		if _, user := rt.userFns[name]; user {
			// A PHP declaration is a closure over the runtime that hoisted it.
			// They come back below, hoisted onto the child from the same AST.
			continue
		}
		child.funcs[name] = entry
	}
	copyEntries(child.constructors, rt.constructors)
	for name, class := range rt.classes {
		child.classes[name] = class
	}
	for name, decl := range rt.interfaces {
		child.interfaces[name] = decl
	}
	// The constants a host froze, not the ones a script defined. FreezeStdlib
	// is where a host says which is which; without it the whole table is
	// treated as the host's, which is what a fork taken before any script ran
	// has anyway.
	source := rt.frozenConsts
	if source == nil {
		source = rt.constants
	}
	for name, value := range source {
		child.constants[name] = value
	}
	if rt.prepare != nil {
		child.prepare = rt.prepare
		rt.prepare(child)
	}
	child.FreezeStdlib()

	// The declarations the tree contributed, replayed rather than copied, so
	// the fork's own tables record where each one came from.
	for program, filename := range rt.hoisted {
		_ = child.hoistOnce(program, filename)
	}
	for _, name := range rt.included {
		child.markIncluded(name)
	}

	// A fork is where a host function that captured the parent would be wrong,
	// so userFns is rebuilt by the hoist above rather than copied.
	return child
}

// copyEntries copies a host symbol table, sharing the entries.
func copyEntries(dst, src map[string]*funcEntry) {
	for name, entry := range src {
		dst[name] = entry
	}
}

// DefaultQueue is how many runs a pool holds waiting for a worker. It is deep
// enough that a burst does not block the caller that produced it and shallow
// enough that a queue this long means the workers are not keeping up, which is
// a thing to find out rather than to absorb.
const DefaultQueue = 1024

// Pool runs PHP on a fixed set of forks, fed from one queue.
//
// A host reaching PHP from a goroutine per request submits a run; a worker
// picks it up on a fork of its own and answers when it is done. Workers are the
// parallelism and the queue is the backpressure: a run that finds every worker
// busy waits its turn rather than starting a runtime of its own, and a queue
// that fills means the caller waits, which is the signal that it should.
//
// A run is expected to be short - a handler answering a request is well under a
// millisecond - so the queue is the right shape rather than a runtime per
// caller. Nothing here is a sync.Pool: the worker count is the contract, and
// sync.Pool drops entries on a collection.
type Pool struct {
	runs    chan poolRun
	workers int
	queue   int

	stop     chan struct{}
	stopOnce sync.Once
	done     sync.WaitGroup
}

// poolRun is one unit of work and the channel its submitter waits on.
type poolRun struct {
	run  func(*Runtime)
	done chan struct{}
}

// NewPool forks rt into workers runtimes, each reading from a queue of depth
// queue, and starts them.
//
// Zero workers is GOMAXPROCS, which is the parallelism the machine has for work
// that is all CPU, and is what a PHP handler doing no IO is; a host whose
// handlers wait on a database wants more. Zero queue is DefaultQueue.
//
// output is called once per worker, for where that worker's runtime writes when
// nothing has been pushed over it. A host wanting a run's output to reach
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
				run.run(worker)
				// Reset after rather than before, so the next run starts on a
				// runtime holding nothing and the values this one built are
				// released now instead of at the next request.
				worker.resetExecution(output(), nil)
				close(run.done)
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
// It answers false when ctx ends first, which for a served request is the
// client leaving: a run still queued is dropped rather than started for nobody,
// and a run already started is left to notice the disconnect itself, because
// stopping one halfway is the handler's decision and connection_aborted is how
// it makes it.
//
// It also answers false once the pool is closed.
func (p *Pool) Submit(ctx context.Context, fn func(*Runtime)) bool {
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}
	run := poolRun{run: fn, done: make(chan struct{})}

	select {
	case p.runs <- run:
	case <-done:
		return false
	case <-p.stop:
		return false
	}

	select {
	case <-run.done:
		return true
	case <-done:
		// Queued or running, it is the worker's now: the run channel is closed
		// by the worker and waiting here any longer would hold the caller for a
		// client that has gone.
		return false
	}
}

// Close stops the workers once the runs already queued have been answered.
func (p *Pool) Close() {
	p.stopOnce.Do(func() {
		close(p.stop)
		close(p.runs)
	})
	p.done.Wait()
}

// InvokeNamed runs the PHP function name with args, on a clean stack holding
// nothing but the arguments.
//
// It is the re-entry a host callback needs: a name is a program counter, and
// everything else about the call comes in through args. A closure is not
// callable this way - it carries the scope it was written in, which belongs to
// the runtime that built it - which is why a host that wants a callback on
// another goroutine asks for one by name.
func (rt *Runtime) InvokeNamed(name string, args ...any) (any, error) {
	entry, ok := rt.lookupEntry(name)
	if !ok {
		return nil, &LookupError{Symbol: name, Reason: "no PHP function of that name is declared"}
	}
	return rt.invokeEntry(entry, args, nil)
}

// DeclaredPrograms answers the programs whose declarations this runtime holds,
// by the filename each was hoisted under. A host building its own forks replays
// them; Fork does it already.
func (rt *Runtime) DeclaredPrograms() map[*model.Program]string {
	out := make(map[*model.Program]string, len(rt.hoisted))
	for program, filename := range rt.hoisted {
		out[program] = filename
	}
	return out
}
