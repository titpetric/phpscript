package runner

import (
	"io"
	goruntime "runtime"

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

// Pool hands out forks, and bounds how many run at once.
//
// A host reaching PHP from a goroutine per request takes one, runs, and gives
// it back. The bound is the point: a fork is a runtime and a runtime is memory,
// so the pool is how many programs may interpret at the same moment rather than
// how many requests may arrive.
//
// It is a buffered channel rather than a sync.Pool because the count is the
// contract. sync.Pool drops entries on a collection, which would turn the cap
// into a suggestion and rebuild a fork mid-load.
type Pool struct {
	free   chan *Runtime
	output func() io.Writer
	size   int
}

// NewPool builds size forks of rt, each writing to what output answers.
//
// Zero size is GOMAXPROCS, which is the parallelism the machine has for work
// that is all CPU, and is what a PHP handler doing no IO is. A host expecting
// its handlers to block on a database wants more.
//
// output is called once per fork. A host that wants a handler's echo to reach
// the response writer pushes one per call instead; see Runtime.PushOutput.
func NewPool(rt *Runtime, size int, output func() io.Writer) *Pool {
	if size <= 0 {
		size = goruntime.GOMAXPROCS(0)
	}
	if output == nil {
		output = func() io.Writer { return io.Discard }
	}

	pool := &Pool{free: make(chan *Runtime, size), output: output, size: size}
	for range size {
		pool.free <- rt.Fork(output())
	}
	return pool
}

// Size answers how many forks the pool holds.
func (p *Pool) Size() int { return p.size }

// Get takes a fork, waiting for one when every fork is busy. A nil done channel
// waits indefinitely; a caller with a request context passes its Done so a
// client that leaves while queued is not waited for.
func (p *Pool) Get(done <-chan struct{}) (*Runtime, bool) {
	select {
	case rt := <-p.free:
		return rt, true
	default:
	}
	select {
	case rt := <-p.free:
		return rt, true
	case <-done:
		return nil, false
	}
}

// Put gives a fork back, dropping whatever the last request left on it and
// keeping what it was forked for.
//
// resetExecution rather than ResetSession: the latter forgets the declarations
// too, which on a fork are the whole point of it, and the next request would
// find nothing to call.
func (p *Pool) Put(rt *Runtime) {
	if rt == nil {
		return
	}
	rt.resetExecution(p.output(), nil)
	p.free <- rt
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
