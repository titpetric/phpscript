package engine

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// scriptExit is what an exit() looks like from here.
//
// The concrete type is runner.ExitError, which this package cannot name:
// runner imports this one. The method is the seam, and it carries the status
// so a future caller that wants the code has it without another interface.
type scriptExit interface {
	ScriptExit() int
}

// execState is one frame stack's worth of VM state, pooled across runs. The
// former vmScratch buffers, the error handlers, the call frames and the pc
// live here so the run loop's helpers are methods instead of closures: a
// closure capturing a loop variable by reference forces that variable onto
// the heap on every Run, and there were five of them.
type execState struct {
	program *Program
	host    Host

	stack       []any
	locals      []any
	initialized []bool
	// extras holds variables a host call introduced under names the compiler
	// never saw. It starts nil and is allocated on first write, so a program
	// that never gains one pays nothing.
	extras     map[string]any
	refWrites  []bool
	iterators  []*iteratorState
	handlers   []errorHandler
	callFrames []callFrame
	deferred   []any
	pc         int

	// walker is created once per pooled state and closes over the state
	// itself, so registering it with a MemoryHost allocates nothing after the
	// state's first use.
	walker func(yield func(any))
}

// release returns the state to the pool with every slot zeroed.
//
// The clears run to capacity rather than to length. The pool holds these
// buffers for the life of the process, so a value left above the high-water
// mark of a later, smaller program stays reachable from the pool and is never
// collected.
func (st *execState) release() {
	st.program, st.host = nil, nil
	st.stack = st.stack[:0]
	clear(st.stack[:cap(st.stack)])
	clear(st.locals[:cap(st.locals)])
	clear(st.initialized[:cap(st.initialized)])
	st.extras = nil
	st.refWrites = nil
	st.iterators = st.iterators[:0]
	clear(st.iterators[:cap(st.iterators)])
	st.handlers = st.handlers[:0]
	clear(st.handlers[:cap(st.handlers)])
	st.callFrames = st.callFrames[:0]
	clear(st.callFrames[:cap(st.callFrames)])
	clear(st.deferred[:cap(st.deferred)])
	st.deferred = st.deferred[:0]
	st.pc = 0
	statePool.Put(st)
}

var statePool = sync.Pool{
	New: func() any {
		st := &execState{
			stack:    make([]any, 0, 32),
			deferred: make([]any, 0, 8),
		}
		st.walker = func(yield func(any)) {
			for _, value := range st.stack {
				yield(value)
			}
			for i := range st.locals {
				if st.initialized[i] {
					yield(st.locals[i])
				}
			}
			for _, frame := range st.callFrames {
				for i := range frame.locals {
					if frame.initialized[i] {
						yield(frame.locals[i])
					}
				}
				// A suspended frame's foreach still holds its listing, so
				// the walk has to reach it or a recursive walk under-reports
				// everything but the innermost loop.
				yieldIterators(yield, frame.iterators)
			}
			yieldIterators(yield, st.iterators)
		}
		return st
	},
}

func (st *execState) pop() (any, error) {
	if len(st.stack) == 0 {
		return nil, fmt.Errorf("operand stack underflow")
	}
	last := len(st.stack) - 1
	value := st.stack[last]
	st.stack[last] = nil
	st.stack = st.stack[:last]
	return value, nil
}

func (st *execState) args(count int) ([]any, error) {
	if count < 0 || count > len(st.stack) {
		return nil, fmt.Errorf("argument stack underflow")
	}
	start := len(st.stack) - count
	values := append([]any(nil), st.stack[start:]...)
	clear(st.stack[start:])
	st.stack = st.stack[:start]
	return values, nil
}

func (st *execState) unwindDeferred(mark int) error {
	var errs []error
	for len(st.deferred) > mark {
		last := len(st.deferred) - 1
		cb := st.deferred[last]
		st.deferred[last] = nil
		st.deferred = st.deferred[:last]
		if cb != nil {
			if callErr := st.host.InvokeCallable(cb); callErr != nil {
				errs = append(errs, callErr)
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (st *execState) handle(runErr error) bool {
	if runErr == nil || len(st.handlers) == 0 {
		return false
	}
	// exit() and die() are not catchable in PHP: a script that ends
	// inside a try ends there. They unwind as an error here only because
	// that is how the VM gets back to the top, so a handler declines
	// them rather than binding them to a catch variable.
	var exiting scriptExit
	if errors.As(runErr, &exiting) {
		return false
	}
	last := len(st.handlers) - 1
	handler := st.handlers[last]
	st.handlers = st.handlers[:last]
	if handler.stackDepth < len(st.stack) {
		clear(st.stack[handler.stackDepth:])
		st.stack = st.stack[:handler.stackDepth]
	}
	for len(st.callFrames) > handler.frameDepth {
		frame := st.callFrames[len(st.callFrames)-1]
		_ = st.unwindDeferred(frame.deferMark)
		st.callFrames = st.callFrames[:len(st.callFrames)-1]
		st.locals = frame.locals
		st.initialized = frame.initialized
		st.extras = frame.extras
		st.refWrites = frame.refWrites
		st.iterators = frame.iterators
	}
	// A catch clause binds the throwable itself, not whatever forwarded
	// it, and the host matches the declared type against the same value.
	for errors.Unwrap(runErr) != nil {
		runErr = errors.Unwrap(runErr)
	}
	var clauses []catchClause
	if handler.group >= 0 {
		clauses = st.program.catchGroups[handler.group]
	}
	for _, clause := range clauses {
		if !st.host.MatchCatch(clause.declaredType, runErr) {
			continue
		}
		if clause.local >= 0 && clause.local < len(st.locals) {
			st.locals[clause.local], st.initialized[clause.local] = st.host.CatchValue(runErr), true
		}
		// A throw out of the clause body is this try's to see through
		// its finally block, but not to catch: PHP does not re-enter a
		// sibling clause. group -1 is a handler with no clauses, armed
		// over the clause bodies and dropped by the jump that leaves
		// them for the finally block.
		st.handlers = append(st.handlers, errorHandler{
			target:     handler.target,
			group:      -1,
			pending:    handler.pending,
			start:      clause.target,
			end:        handler.target - 1,
			stackDepth: handler.stackDepth,
			frameDepth: handler.frameDepth,
		})
		st.pc = clause.target
		return true
	}
	// No clause declared a type that matches. The error keeps
	// propagating, but this try's finally block still has to run first,
	// so park it and let opRethrow pick it up on the other side.
	if handler.pending < 0 || handler.pending >= len(st.locals) {
		return false
	}
	st.locals[handler.pending], st.initialized[handler.pending] = runErr, true
	st.pc = handler.target
	return true
}

// iterator returns the live foreach state numbered n, or nil.
func (st *execState) iterator(n int) *iteratorState {
	if n < len(st.iterators) {
		return st.iterators[n]
	}
	return nil
}

// setIterator grows the slice to hold iterator n. The compiler numbers
// iterators per function from zero, so the slice stays as small as the
// deepest loop nest.
func (st *execState) setIterator(n int, it *iteratorState) {
	for len(st.iterators) <= n {
		st.iterators = append(st.iterators, nil)
	}
	st.iterators[n] = it
}

type iteratorState struct {
	entries []Entry
	index   int
	// source and key are what a by-reference loop writes its target back into:
	// the container the entries came from, and the key of the entry currently
	// bound to the loop variable.
	source any
	key    any
}

// yieldIterators hands a live-value walker everything one frame's foreach state
// is holding: the container being walked and every entry taken off it.
func yieldIterators(yield func(any), iterators []*iteratorState) {
	for _, iterator := range iterators {
		if iterator == nil {
			continue
		}
		yield(iterator.source)
		for _, entry := range iterator.entries {
			yield(entry.Key)
			yield(entry.Value)
		}
	}
}

type errorHandler struct {
	// target is the finally block of the try, entered either by a matching
	// catch clause falling through it or by an unmatched error on its way out.
	target int
	// group indexes Program.catchGroups; pending is the hidden local an
	// unmatched error waits in while the finally block runs.
	group      int
	pending    int
	start      int
	end        int
	stackDepth int
	frameDepth int
}

type callFrame struct {
	returnPC    int
	locals      []any
	initialized []bool
	extras      map[string]any
	refWrites   []bool

	// iterators is the caller's live foreach state. The compiler numbers
	// iterators per function, so a callee reuses the numbers its caller is
	// standing in, and a recursive call reuses them exactly. Saving the slice
	// here and handing the callee an empty one is what keeps a foreach that
	// calls a function from being closed by the call it made.
	iterators []*iteratorState

	// deferMark is the caller's boundary in scratch.deferred; opReturn unwinds
	// the registrations above it, LIFO, before control leaves the frame.
	deferMark int
}

// MemoryHost is an optional Host extension for memory accounting, discovered
// by type assertion like the other optional host capabilities. A host that
// implements it can enumerate the VM's live values while Run executes, and
// have execution interrupted when its memory limit is exceeded.
type MemoryHost interface {
	// MemoryCheckInterval returns the number of instructions between memory
	// checks; zero disables checking. Live-value registration happens either
	// way, so usage queries still see VM state.
	MemoryCheckInterval() int
	// PushLiveWalker registers an enumerator over every live VM value;
	// PopLiveWalker removes it. Calls nest across nested Run invocations.
	PushLiveWalker(func(yield func(any)))
	PopLiveWalker()
	// CheckMemory reports the limit error, if any.
	CheckMemory() error
}

// localSeed is one local written into a frame before it starts running: the
// captures and the arguments of a closure call.
type localSeed struct {
	slot  int
	value any
}

// FrameLocals is the engine's view of the frame in flight, bound to the host
// once per run instead of copied around every call. Snapshot materialises the
// frame's variables only when a callee actually needs them, and WriteBack
// applies the mutations such a call made. The snapshot-before-call ordering
// the old map handshake had is preserved because the host decides both
// moments: it snapshots before the binding body runs and writes back after.
type FrameLocals interface {
	Snapshot() map[string]any
	WriteBack(vars map[string]any)
}

// hostFrame is the optional Host capability that receives the frame handle.
type hostFrame interface {
	BindFrame(FrameLocals)
	TakeFrame() FrameLocals
}

// Snapshot builds the map bindHostLocals used to build per host call: every
// initialised, non-hidden local, then the extras a host call introduced.
func (st *execState) Snapshot() map[string]any {
	vars := make(map[string]any, len(st.program.localNames)+len(st.extras))
	for i, name := range st.program.localNames {
		if st.initialized[i] && (len(name) == 0 || name[0] != 0) {
			vars[name] = st.locals[i]
		}
	}
	for name, value := range st.extras {
		if _, ok := vars[name]; !ok {
			vars[name] = value
		}
	}
	return vars
}

// WriteBack writes host-visible variables into their slots, respecting the
// by-reference marks the way the old applyHostLocals did.
func (st *execState) WriteBack(vars map[string]any) {
	st.extras = applyNamedValues(st.program, st.locals, st.initialized, st.extras, vars, st.refWrites)
}

// Run executes a previously validated flat instruction stream.
func Run(program *Program, host Host) error {
	if program == nil {
		return nil
	}
	if registrar, ok := host.(interface{ RegisterClass(*model.Class) }); ok {
		for _, class := range program.classes {
			registrar.RegisterClass(class)
		}
	}
	return run(program, host, 0, nil, nil)
}

// run executes one frame, starting at entryPC with seeds already written into
// its locals. The value the frame returns is reported through result, which is
// nil for the top-level frame because nothing consumes it.
//
// A closure call re-enters here rather than pushing a call frame on the running
// loop: the call arrives from a host binding (usort() invoking its comparator),
// not from an instruction, so there is no loop to push onto.
func run(program *Program, host Host, entryPC int, seeds []localSeed, result *any) (err error) {
	st := statePool.Get().(*execState)
	st.program, st.host = program, host
	st.pc = entryPC
	nlocal := len(program.localNames)
	if cap(st.locals) < nlocal {
		st.locals = make([]any, nlocal)
		st.initialized = make([]bool, nlocal)
	}
	st.locals = st.locals[:nlocal]
	st.initialized = st.initialized[:nlocal]
	clear(st.locals)
	clear(st.initialized)
	for _, seed := range seeds {
		if seed.slot >= 0 && seed.slot < len(st.locals) {
			st.locals[seed.slot], st.initialized[seed.slot] = seed.value, true
		}
	}

	entryDeferMark := len(st.deferred)
	defer func() {
		unwindErr := st.unwindDeferred(entryDeferMark)
		pcAt := st.pc
		st.release()
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("flatstack: VM panic at pc %d: %v", pcAt, recovered)
		} else if err == nil && unwindErr != nil {
			err = unwindErr
		}
	}()

	memHost, hasMemHost := host.(MemoryHost)
	memInterval, memTick := 0, 0
	if hasMemHost {
		memInterval = memHost.MemoryCheckInterval()
		// The pooled state carries its walker, created once in statePool.New,
		// so registration allocates nothing after the state's first use.
		memHost.PushLiveWalker(st.walker)
		defer memHost.PopLiveWalker()
	}

	// The frame handle replaces the per-call locals copy: the host holds it
	// for the whole run and snapshots only when a callee needs the scope. A
	// nested run (a closure invoked from a binding) binds its own state here
	// and puts the caller's back on the way out, which is what the old
	// restoreHostLocals defer did.
	if binder, ok := host.(hostFrame); ok {
		prev := binder.TakeFrame()
		binder.BindFrame(st)
		defer binder.BindFrame(prev)
	}

	for st.pc < len(program.code) {
		if memInterval > 0 {
			if memTick++; memTick >= memInterval {
				memTick = 0
				if memErr := memHost.CheckMemory(); memErr != nil {
					if st.handle(memErr) {
						continue
					}
					return memErr
				}
			}
		}
		inst := program.code[st.pc]
		switch inst.op {
		case opPushConst:
			st.stack = append(st.stack, program.constants[inst.a])
		case opPop:
			if _, err = st.pop(); err != nil {
				return err
			}
		case opDup:
			if len(st.stack) == 0 {
				return fmt.Errorf("flatstack: pc %d: duplicate st.stack underflow", st.pc)
			}
			st.stack = append(st.stack, st.stack[len(st.stack)-1])
		case opLoad:
			st.stack = append(st.stack, loadLocal(host, program, st.locals, st.initialized, st.extras, inst.a))
		case opLoadConst:
			name := program.localNames[inst.a]
			// A scope value of the same name wins, which is how the magic
			// constants set per frame answer before the constant table.
			if st.initialized[inst.a] {
				st.stack = append(st.stack, st.locals[inst.a])
				break
			}
			if extra, ok := st.extras[name]; ok {
				st.stack = append(st.stack, extra)
				break
			}
			value, constErr := host.Constant(name)
			if constErr != nil {
				// Through handle, so a catch clause binds it the way it binds
				// an error a binding returned.
				if st.handle(constErr) {
					continue
				}
				return constErr
			}
			st.stack = append(st.stack, value)
		case opClosure:
			def := program.closures[inst.a]
			captured := make([]localSeed, 0, len(def.captures)+1)
			for _, slot := range def.captures {
				captured = append(captured, localSeed{
					slot:  slot,
					value: loadLocal(host, program, st.locals, st.initialized, st.extras, slot),
				})
			}
			// An unbound `$this` is left out rather than captured as null, so
			// the closure body reads it the way any other unset local is read.
			if def.thisSlot >= 0 && st.initialized[def.thisSlot] {
				captured = append(captured, localSeed{slot: def.thisSlot, value: st.locals[def.thisSlot]})
			}
			st.stack = append(st.stack, closureValue(program, host, def, captured))
		case opStore:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if inst.name != "" && inst.name != "=" {
				var current any
				if st.initialized[inst.a] {
					current = st.locals[inst.a]
				} else {
					current = host.Lookup(program.localNames[inst.a])
				}
				operator := inst.name[:len(inst.name)-1]
				updated, binaryErr := host.Binary(operator, current, value)
				if binaryErr != nil {
					// A compound assignment can fail the same way the binary
					// operator can ($x >>= -1), and it is as catchable.
					if st.handle(binaryErr) {
						continue
					}
					return binaryErr
				}
				value = updated
			}
			if identifiable, ok := value.(interface{ SetID(string) }); ok && inst.extra != "" {
				identifiable.SetID(inst.extra)
			}
			if host.SetGlobal(program.localNames[inst.a], value) {
				// The host claimed the name — a superglobal — so the store is
				// request state, not frame state.
				st.initialized[inst.a] = false
			} else {
				st.locals[inst.a], st.initialized[inst.a] = value, true
			}
			if inst.b != 0 {
				st.stack = append(st.stack, value)
			}
		case opArray:
			items := make([]model.ArrayItemValue, inst.a)
			for i := inst.a - 1; i >= 0; i-- {
				value, popErr := st.pop()
				if popErr != nil {
					return popErr
				}
				key, popErr := st.pop()
				if popErr != nil {
					return popErr
				}
				items[i] = model.ArrayItemValue{Key: key, Val: value}
			}
			st.stack = append(st.stack, host.Array(items))
		case opIndex:
			index, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			st.stack = append(st.stack, host.Index(base, index))
		case opSetIndex:
			var index any
			var popErr error
			if inst.b == 0 {
				index, popErr = st.pop()
				if popErr != nil {
					return popErr
				}
			}
			base, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if err = host.SetIndex(base, index, value, inst.b != 0, inst.name); err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
			if inst.c != 0 {
				st.stack = append(st.stack, value)
			}
		case opIncDecLocal:
			var current any
			if st.initialized[inst.a] {
				current = st.locals[inst.a]
			} else {
				current = host.Lookup(program.localNames[inst.a])
			}
			next := phpval.Increment(current)
			if inst.name == "--" {
				next = phpval.Decrement(current)
			}
			st.locals[inst.a], st.initialized[inst.a] = next, true
			if inst.b != 0 {
				st.stack = append(st.stack, current)
			} else {
				st.stack = append(st.stack, next)
			}
		case opIncDecIndex:
			index, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			current := host.Index(base, index)
			next := phpval.Increment(current)
			if inst.name == "--" {
				next = phpval.Decrement(current)
			}
			if err = host.SetIndex(base, index, next, false, "="); err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
			if inst.b != 0 {
				st.stack = append(st.stack, current)
			} else {
				st.stack = append(st.stack, next)
			}
		case opBinary:
			right, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			left, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			value, binaryErr := host.Binary(inst.name, left, right)
			if binaryErr != nil {
				if st.handle(binaryErr) {
					continue
				}
				return binaryErr
			}
			st.stack = append(st.stack, value)
		case opUnary:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			value, err = host.Unary(inst.name, value)
			if err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
			st.stack = append(st.stack, value)
		case opCast:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			st.stack = append(st.stack, host.Cast(inst.name, value))
		case opClassConst:
			value, constErr := host.ClassConst(inst.name, inst.extra)
			if constErr != nil {
				if st.handle(constErr) {
					continue
				}
				return constErr
			}
			st.stack = append(st.stack, value)
		case opDefineConst:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			host.SetConstant(inst.name, value)
		case opTruthy:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			st.stack = append(st.stack, host.Truthy(value))
		case opJump:
			// A jump out of a try's pc range discards its handler; that is how
			// leaving the region disarms the catch. The range only means
			// anything in the frame that armed it: a callee's pcs lie outside
			// the caller's try body, so a jump there - an if, a loop, the skip
			// over an inline closure - must leave the caller's st.handlers alone.
			for len(st.handlers) > 0 {
				handler := st.handlers[len(st.handlers)-1]
				if handler.frameDepth != len(st.callFrames) {
					break
				}
				if inst.target >= handler.start && inst.target <= handler.end {
					break
				}
				st.handlers = st.handlers[:len(st.handlers)-1]
			}
			st.pc = inst.target
			continue
		case opJumpFalse, opJumpTrue:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			truthy := host.Truthy(value)
			if (inst.op == opJumpFalse && !truthy) || (inst.op == opJumpTrue && truthy) {
				st.pc = inst.target
				continue
			}
		case opRef:
			// The setter writes the frame the call was made from; a user
			// function called in between installs its own st.locals, and this one
			// keeps pointing at the caller's.
			if st.refWrites == nil {
				st.refWrites = make([]bool, len(st.locals))
			}
			frame, frameInitialized, frameRefWrites, slot := st.locals, st.initialized, st.refWrites, inst.a
			st.stack = append(st.stack, func(value any) {
				frame[slot], frameInitialized[slot] = value, true
				frameRefWrites[slot] = true
			})
		case opCall, opConstruct:
			arguments, argErr := st.args(inst.a)
			if argErr != nil {
				return argErr
			}
			var value any
			if inst.op == opCall {
				if def, ok := lookupUserFunc(program, inst.name); ok {
					st.callFrames = append(st.callFrames, callFrame{
						returnPC:    st.pc,
						locals:      st.locals,
						initialized: st.initialized,
						extras:      st.extras,
						refWrites:   st.refWrites,
						iterators:   st.iterators,
						deferMark:   len(st.deferred),
					})
					st.locals = make([]any, len(program.localNames))
					st.initialized = make([]bool, len(program.localNames))
					st.extras = nil
					st.refWrites = nil
					st.iterators = nil
					for i, slot := range def.paramSlots {
						if i < len(arguments) {
							st.locals[slot], st.initialized[slot] = arguments[i], true
						}
					}
					st.pc = def.entryPC
					continue
				}
				value, err = host.Call(inst.name, inst.extra, arguments)
			} else {
				value, err = host.Construct(inst.name, arguments)
			}
			clear(st.refWrites)
			if err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
			st.stack = append(st.stack, value)
		case opCallMethod:
			arguments, argErr := st.args(inst.a)
			if argErr != nil {
				return argErr
			}
			receiver, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if obj, ok := receiver.(*model.Object); ok && obj.Class != nil {
				key := obj.Class.Name + "::" + inst.name
				if def, ok := lookupUserFunc(program, key); ok {
					st.callFrames = append(st.callFrames, callFrame{
						returnPC:    st.pc,
						locals:      st.locals,
						initialized: st.initialized,
						extras:      st.extras,
						refWrites:   st.refWrites,
						iterators:   st.iterators,
						deferMark:   len(st.deferred),
					})
					st.locals = make([]any, len(program.localNames))
					st.initialized = make([]bool, len(program.localNames))
					st.extras = nil
					st.refWrites = nil
					st.iterators = nil
					// paramSlots[0] is the receiver slot; arguments fill the rest,
					// shifted by one, without materialising a combined slice.
					for i, slot := range def.paramSlots {
						switch {
						case i == 0:
							st.locals[slot], st.initialized[slot] = receiver, true
						case i-1 < len(arguments):
							st.locals[slot], st.initialized[slot] = arguments[i-1], true
						}
					}
					st.pc = def.entryPC
					continue
				}
			}
			value, callErr := host.CallMethod(receiver, inst.name, arguments)
			clear(st.refWrites)
			if callErr != nil {
				if st.handle(callErr) {
					continue
				}
				return callErr
			}
			st.stack = append(st.stack, value)
		case opGetProperty:
			receiver, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			st.stack = append(st.stack, host.GetProperty(receiver, inst.name))
		case opSetProperty:
			receiver, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if err = host.SetProperty(receiver, inst.name, value, inst.extra); err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
			if inst.b != 0 {
				st.stack = append(st.stack, value)
			}
		case opEcho:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if err = host.Echo(value); err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
		case opIterInit:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			st.setIterator(inst.a, &iteratorState{entries: host.Entries(value), source: value})
		case opIterNext:
			iterator := st.iterator(inst.a)
			if iterator == nil || iterator.index >= len(iterator.entries) {
				st.pc = inst.target
				continue
			}
			entry := iterator.entries[iterator.index]
			iterator.index++
			iterator.key = entry.Key
			if inst.b >= 0 {
				st.locals[inst.b], st.initialized[inst.b] = entry.Key, true
			}
			st.locals[inst.c], st.initialized[inst.c] = entry.Value, true
		case opIterSet:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			iterator := st.iterator(inst.a)
			if iterator == nil {
				return fmt.Errorf("flatstack: pc %d: write-back to a closed iterator", st.pc)
			}
			if err = host.SetEntry(iterator.source, iterator.key, value); err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
		case opUnsetLocal:
			st.locals[inst.a], st.initialized[inst.a] = nil, false
		case opUnsetIndex:
			index, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if err = host.UnsetIndex(base, index); err != nil {
				if st.handle(err) {
					continue
				}
				return err
			}
		case opCopyValue:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			st.stack = append(st.stack, model.CopyValue(value))
		case opIterClose:
			if inst.a < len(st.iterators) {
				st.iterators[inst.a] = nil
			}
		case opTryPush:
			st.handlers = append(st.handlers, errorHandler{
				target:     inst.target,
				group:      inst.a,
				pending:    inst.c,
				start:      st.pc + 1,
				end:        inst.b,
				stackDepth: len(st.stack),
				frameDepth: len(st.callFrames),
			})
		case opTryPop:
			if len(st.handlers) == 0 {
				return fmt.Errorf("flatstack: pc %d: exception handler underflow", st.pc)
			}
			st.handlers = st.handlers[:len(st.handlers)-1]
		case opThrow:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			// A thrown throwable is already an error and propagates as
			// itself, so a catch clause binds the object rather than a
			// rendering of it. A bare value still renders.
			throwErr := host.Throw(value)
			if st.handle(throwErr) {
				continue
			}
			return throwErr
		case opRethrow:
			// Sits after every finally block. It fires only for an error no
			// catch clause of that try matched, which the handler parked here
			// so the finally block could run on the way out.
			if !st.initialized[inst.a] {
				break
			}
			pending, _ := st.locals[inst.a].(error)
			st.locals[inst.a], st.initialized[inst.a] = nil, false
			if pending == nil {
				break
			}
			if st.handle(pending) {
				continue
			}
			return pending
		case opReturn:
			retVal, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if len(st.callFrames) > 0 {
				lastFrame := st.callFrames[len(st.callFrames)-1]
				if unwindErr := st.unwindDeferred(lastFrame.deferMark); unwindErr != nil {
					if st.handle(unwindErr) {
						continue
					}
					return unwindErr
				}
				st.callFrames = st.callFrames[:len(st.callFrames)-1]
				st.pc = lastFrame.returnPC
				st.locals = lastFrame.locals
				st.initialized = lastFrame.initialized
				st.extras = lastFrame.extras
				st.refWrites = lastFrame.refWrites
				st.iterators = lastFrame.iterators
				st.stack = append(st.stack, retVal)
			} else {
				if unwindErr := st.unwindDeferred(entryDeferMark); unwindErr != nil {
					if st.handle(unwindErr) {
						continue
					}
					return unwindErr
				}
				if result != nil {
					*result = retVal
				}
				return nil
			}
		case opEnsureArray:
			value, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			if _, ok := value.(*model.Array); !ok && phpEmptyContainer(value) {
				value = host.Array(nil)
			}
			st.stack = append(st.stack, value)
		case opVivifyIndex:
			index, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			value := host.Index(base, index)
			if _, ok := value.(*model.Array); !ok && phpEmptyContainer(value) {
				value = host.Array(nil)
				if err = host.SetIndex(base, index, value, false, "="); err != nil {
					if st.handle(err) {
						continue
					}
					return err
				}
			}
			st.stack = append(st.stack, value)
		case opVivifyProperty:
			receiver, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			value := host.GetProperty(receiver, inst.name)
			if _, ok := value.(*model.Array); !ok && phpEmptyContainer(value) {
				value = host.Array(nil)
				if err = host.SetProperty(receiver, inst.name, value, "="); err != nil {
					if st.handle(err) {
						continue
					}
					return err
				}
			}
			st.stack = append(st.stack, value)
		case opInclude:
			pathValue, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			vars := st.Snapshot()
			includer, ok := host.(interface {
				Include(path any, keyword string, once bool, vars map[string]any) (any, map[string]any, error)
			})
			if !ok {
				return fmt.Errorf("flatstack: pc %d: host does not implement include", st.pc)
			}
			value, exported, includeErr := includer.Include(pathValue, inst.name, inst.a != 0, vars)
			if includeErr != nil {
				if st.handle(includeErr) {
					continue
				}
				return includeErr
			}
			st.extras = applyNamedValues(program, st.locals, st.initialized, st.extras, exported, st.refWrites)
			st.stack = append(st.stack, value)
		case opDefer:
			callable, popErr := st.pop()
			if popErr != nil {
				return popErr
			}
			st.deferred = append(st.deferred, callable)
			st.stack = append(st.stack, nil)
		default:
			return fmt.Errorf("flatstack: pc %d: invalid opcode %d", st.pc, inst.op)
		}
		st.pc++
	}
	return nil
}

// loadLocal reads a frame slot the way opLoad does: the local if the frame set
// it, otherwise a variable a host binding introduced, otherwise whatever the
// host knows under that name (a global or a constant).
func loadLocal(host Host, program *Program, locals []any, initialized []bool, extras map[string]any, slot int) any {
	if initialized[slot] {
		return locals[slot]
	}
	if extra, ok := extras[program.localNames[slot]]; ok {
		return extra
	}
	return host.Lookup(program.localNames[slot])
}

// closureValue turns a compiled closure into the func(...any) (any, error)
// shape Runtime.Callable reduces every PHP callable to, so a binding invokes a
// bytecode closure exactly as it invokes an interpreted one.
//
// captured is the snapshot taken where the closure value was created. Each call
// gets a fresh frame seeded with it, so one call cannot see another's writes,
// and the closure keeps working after the frame it was written in is gone.
func closureValue(program *Program, host Host, def closureDef, captured []localSeed) func(...any) (any, error) {
	return func(args ...any) (any, error) {
		seeds := make([]localSeed, 0, len(captured)+len(def.paramSlots))
		seeds = append(seeds, captured...)
		// Parameters are seeded after the captures, so a parameter of the same
		// name shadows the capture, as it does in PHP. An argument the caller
		// omitted binds null, matching the interpreter's bindParams.
		for i, slot := range def.paramSlots {
			var value any
			if i < len(args) {
				value = args[i]
			}
			seeds = append(seeds, localSeed{slot: slot, value: value})
		}
		// run binds its own frame handle and puts the caller's back on the
		// way out, so the snapshot usort() took before invoking this
		// comparator stays the one usort()'s write-back sees.
		var result any
		if err := run(program, host, def.entryPC, seeds, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

// applyNamedValues writes host-visible variables back into their slots. A slot
// marked in refWrites keeps what the by-reference setter put there: names is a
// snapshot from before the call, so it still carries the old value. A name the
// compiler never saw goes to extras, which is allocated here on first use and
// handed back to the caller.
func applyNamedValues(program *Program, locals []any, initialized []bool, extras map[string]any, names map[string]any, refWrites []bool) map[string]any {
	for name, value := range names {
		if len(name) == 0 || name[0] == 0 {
			continue
		}
		if i, ok := program.nameSlots[name]; ok {
			if i >= len(refWrites) || !refWrites[i] {
				locals[i], initialized[i] = value, true
			}
			continue
		}
		if extras == nil {
			extras = make(map[string]any)
		}
		extras[name] = value
	}
	return extras
}

func lookupUserFunc(program *Program, key string) (userFuncDef, bool) {
	if def, ok := program.userFuncs[key]; ok {
		return def, true
	}
	if def, ok := program.userFuncsFold[strings.ToLower(key)]; ok {
		return def, true
	}
	return userFuncDef{}, false
}

func phpEmptyContainer(value any) bool {
	return value == nil
}
