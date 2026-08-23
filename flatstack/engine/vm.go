package engine

import (
	"errors"
	"fmt"
	"strings"
	"sync"

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

type vmScratch struct {
	stack       []any
	locals      []any
	initialized []bool
}

// release returns the scratch buffers to the pool with every slot zeroed.
//
// The clears run to capacity rather than to length. The pool holds these
// buffers for the life of the process, so a value left above the high-water
// mark of a later, smaller program stays reachable from the pool and is never
// collected. stack is passed back in because append may have moved it.
func (s *vmScratch) release(stack []any) {
	s.stack = stack[:0]
	clear(s.stack[:cap(s.stack)])
	clear(s.locals[:cap(s.locals)])
	clear(s.initialized[:cap(s.initialized)])
	scratchPool.Put(s)
}

var scratchPool = sync.Pool{
	New: func() any {
		return &vmScratch{stack: make([]any, 0, 32)}
	},
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

type errorHandler struct {
	target     int
	local      int
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

// Run executes a previously validated flat instruction stream.
func Run(program *Program, host Host) (err error) {
	if program == nil {
		return nil
	}
	pc := 0
	scratch := scratchPool.Get().(*vmScratch)
	stack := scratch.stack[:0]
	nlocal := len(program.localNames)
	if cap(scratch.locals) < nlocal {
		scratch.locals = make([]any, nlocal)
		scratch.initialized = make([]bool, nlocal)
	}
	locals := scratch.locals[:nlocal]
	initialized := scratch.initialized[:nlocal]
	clear(locals)
	clear(initialized)
	if registrar, ok := host.(interface{ RegisterClass(*model.Class) }); ok {
		for _, class := range program.classes {
			registrar.RegisterClass(class)
		}
	}
	extras := map[string]any{}
	iterators := make(map[int]*iteratorState)
	var handlers []errorHandler
	var callFrames []callFrame
	defer func() {
		scratch.release(stack)
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("flatstack: VM panic at pc %d: %v", pc, recovered)
		}
	}()

	pop := func() (any, error) {
		if len(stack) == 0 {
			return nil, fmt.Errorf("operand stack underflow")
		}
		last := len(stack) - 1
		value := stack[last]
		stack[last] = nil
		stack = stack[:last]
		return value, nil
	}
	args := func(count int) ([]any, error) {
		if count < 0 || count > len(stack) {
			return nil, fmt.Errorf("argument stack underflow")
		}
		start := len(stack) - count
		values := append([]any(nil), stack[start:]...)
		clear(stack[start:])
		stack = stack[:start]
		return values, nil
	}
	handle := func(runErr error) bool {
		if runErr == nil || len(handlers) == 0 {
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
		last := len(handlers) - 1
		handler := handlers[last]
		handlers = handlers[:last]
		if handler.stackDepth < len(stack) {
			clear(stack[handler.stackDepth:])
			stack = stack[:handler.stackDepth]
		}
		for len(callFrames) > handler.frameDepth {
			frame := callFrames[len(callFrames)-1]
			callFrames = callFrames[:len(callFrames)-1]
			locals = frame.locals
			initialized = frame.initialized
			extras = frame.extras
			if extras == nil {
				extras = map[string]any{}
			}
		}
		for errors.Unwrap(runErr) != nil {
			runErr = errors.Unwrap(runErr)
		}
		if handler.local >= 0 && handler.local < len(locals) {
			locals[handler.local], initialized[handler.local] = runErr, true
		}
		pc = handler.target
		return true
	}

	memHost, hasMemHost := host.(MemoryHost)
	memInterval, memTick := 0, 0
	if hasMemHost {
		memInterval = memHost.MemoryCheckInterval()
		// The walker closes over the loop variables themselves, so slice
		// reassignment by append/opCall/opReturn stays visible to it.
		memHost.PushLiveWalker(func(yield func(any)) {
			for _, value := range stack {
				yield(value)
			}
			for i := range locals {
				if initialized[i] {
					yield(locals[i])
				}
			}
			for _, frame := range callFrames {
				for i := range frame.locals {
					if frame.initialized[i] {
						yield(frame.locals[i])
					}
				}
			}
			for _, iterator := range iterators {
				yield(iterator.source)
				for _, entry := range iterator.entries {
					yield(entry.Key)
					yield(entry.Value)
				}
			}
		})
		defer memHost.PopLiveWalker()
	}

	for pc < len(program.code) {
		if memInterval > 0 {
			if memTick++; memTick >= memInterval {
				memTick = 0
				if memErr := memHost.CheckMemory(); memErr != nil {
					if handle(memErr) {
						continue
					}
					return memErr
				}
			}
		}
		inst := program.code[pc]
		switch inst.op {
		case opPushConst:
			stack = append(stack, program.constants[inst.a])
		case opPop:
			if _, err = pop(); err != nil {
				return err
			}
		case opDup:
			if len(stack) == 0 {
				return fmt.Errorf("flatstack: pc %d: duplicate stack underflow", pc)
			}
			stack = append(stack, stack[len(stack)-1])
		case opLoad:
			if initialized[inst.a] {
				stack = append(stack, locals[inst.a])
			} else if extra, ok := extras[program.localNames[inst.a]]; ok {
				stack = append(stack, extra)
			} else {
				stack = append(stack, host.Lookup(program.localNames[inst.a]))
			}
		case opStore:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if inst.name != "" && inst.name != "=" {
				current := host.Lookup(program.localNames[inst.a])
				if initialized[inst.a] {
					current = locals[inst.a]
				}
				operator := inst.name[:len(inst.name)-1]
				value, err = host.Binary(operator, current, value)
				if err != nil {
					return err
				}
			}
			if identifiable, ok := value.(interface{ SetID(string) }); ok && inst.extra != "" {
				identifiable.SetID(inst.extra)
			}
			locals[inst.a], initialized[inst.a] = value, true
			if inst.b != 0 {
				stack = append(stack, value)
			}
		case opArray:
			items := make([]model.ArrayItemValue, inst.a)
			for i := inst.a - 1; i >= 0; i-- {
				value, popErr := pop()
				if popErr != nil {
					return popErr
				}
				key, popErr := pop()
				if popErr != nil {
					return popErr
				}
				items[i] = model.ArrayItemValue{Key: key, Val: value}
			}
			stack = append(stack, host.Array(items))
		case opIndex:
			index, popErr := pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := pop()
			if popErr != nil {
				return popErr
			}
			stack = append(stack, host.Index(base, index))
		case opSetIndex:
			var index any
			var popErr error
			if inst.b == 0 {
				index, popErr = pop()
				if popErr != nil {
					return popErr
				}
			}
			base, popErr := pop()
			if popErr != nil {
				return popErr
			}
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if err = host.SetIndex(base, index, value, inst.b != 0, inst.name); err != nil {
				if handle(err) {
					continue
				}
				return err
			}
			if inst.c != 0 {
				stack = append(stack, value)
			}
		case opIncDecLocal:
			current := host.Lookup(program.localNames[inst.a])
			if initialized[inst.a] {
				current = locals[inst.a]
			}
			operator := "+"
			if inst.name == "--" {
				operator = "-"
			}
			next, binaryErr := host.Binary(operator, current, int64(1))
			if binaryErr != nil {
				if handle(binaryErr) {
					continue
				}
				return binaryErr
			}
			locals[inst.a], initialized[inst.a] = next, true
			if inst.b != 0 {
				stack = append(stack, current)
			} else {
				stack = append(stack, next)
			}
		case opIncDecIndex:
			index, popErr := pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := pop()
			if popErr != nil {
				return popErr
			}
			current := host.Index(base, index)
			operator := "+"
			if inst.name == "--" {
				operator = "-"
			}
			next, binaryErr := host.Binary(operator, current, int64(1))
			if binaryErr != nil {
				if handle(binaryErr) {
					continue
				}
				return binaryErr
			}
			if err = host.SetIndex(base, index, next, false, "="); err != nil {
				if handle(err) {
					continue
				}
				return err
			}
			if inst.b != 0 {
				stack = append(stack, current)
			} else {
				stack = append(stack, next)
			}
		case opBinary:
			right, popErr := pop()
			if popErr != nil {
				return popErr
			}
			left, popErr := pop()
			if popErr != nil {
				return popErr
			}
			value, binaryErr := host.Binary(inst.name, left, right)
			if binaryErr != nil {
				if handle(binaryErr) {
					continue
				}
				return binaryErr
			}
			stack = append(stack, value)
		case opUnary:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			value, err = host.Unary(inst.name, value)
			if err != nil {
				if handle(err) {
					continue
				}
				return err
			}
			stack = append(stack, value)
		case opTruthy:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			stack = append(stack, host.Truthy(value))
		case opJump:
			for len(handlers) > 0 {
				handler := handlers[len(handlers)-1]
				if inst.target >= handler.start && inst.target <= handler.end {
					break
				}
				handlers = handlers[:len(handlers)-1]
			}
			pc = inst.target
			continue
		case opJumpFalse, opJumpTrue:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			truthy := host.Truthy(value)
			if (inst.op == opJumpFalse && !truthy) || (inst.op == opJumpTrue && truthy) {
				pc = inst.target
				continue
			}
		case opRef:
			// The setter writes the frame the call was made from; a user
			// function called in between installs its own locals, and this one
			// keeps pointing at the caller's.
			frame, frameInitialized, slot := locals, initialized, inst.a
			stack = append(stack, func(value any) {
				frame[slot], frameInitialized[slot] = value, true
			})
		case opCall, opConstruct:
			arguments, argErr := args(inst.a)
			if argErr != nil {
				return argErr
			}
			bindHostLocals(host, program, locals, initialized, extras)
			var value any
			if inst.op == opCall {
				if def, ok := lookupUserFunc(program.userFuncs, inst.name); ok {
					callFrames = append(callFrames, callFrame{
						returnPC:    pc,
						locals:      locals,
						initialized: initialized,
						extras:      extras,
					})
					locals = make([]any, len(program.localNames))
					initialized = make([]bool, len(program.localNames))
					extras = map[string]any{}
					for i, paramName := range def.params {
						if i < len(arguments) {
							slot := -1
							for s, name := range program.localNames {
								if name == paramName {
									slot = s
									break
								}
							}
							if slot >= 0 {
								locals[slot], initialized[slot] = arguments[i], true
							}
						}
					}
					pc = def.entryPC
					continue
				}
				value, err = host.Call(inst.name, inst.extra, arguments)
				applyHostLocals(host, program, locals, initialized, extras)
			} else {
				value, err = host.Construct(inst.name, arguments)
				applyHostLocals(host, program, locals, initialized, extras)
			}
			if err != nil {
				if handle(err) {
					continue
				}
				return err
			}
			stack = append(stack, value)
		case opCallMethod:
			arguments, argErr := args(inst.a)
			if argErr != nil {
				return argErr
			}
			bindHostLocals(host, program, locals, initialized, extras)
			receiver, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if obj, ok := receiver.(*model.Object); ok && obj.Class != nil {
				key := obj.Class.Name + "::" + inst.name
				if def, ok := lookupUserFunc(program.userFuncs, key); ok {
					callFrames = append(callFrames, callFrame{
						returnPC:    pc,
						locals:      locals,
						initialized: initialized,
						extras:      extras,
					})
					locals = make([]any, len(program.localNames))
					initialized = make([]bool, len(program.localNames))
					extras = map[string]any{}
					bound := append([]any{receiver}, arguments...)
					for i, paramName := range def.params {
						if i >= len(bound) {
							break
						}
						for s, name := range program.localNames {
							if name == paramName {
								locals[s], initialized[s] = bound[i], true
								break
							}
						}
					}
					pc = def.entryPC
					continue
				}
			}
			value, callErr := host.CallMethod(receiver, inst.name, arguments)
			applyHostLocals(host, program, locals, initialized, extras)
			if callErr != nil {
				if handle(callErr) {
					continue
				}
				return callErr
			}
			stack = append(stack, value)
		case opGetProperty:
			receiver, popErr := pop()
			if popErr != nil {
				return popErr
			}
			stack = append(stack, host.GetProperty(receiver, inst.name))
		case opSetProperty:
			receiver, popErr := pop()
			if popErr != nil {
				return popErr
			}
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if err = host.SetProperty(receiver, inst.name, value, inst.extra); err != nil {
				if handle(err) {
					continue
				}
				return err
			}
			if inst.b != 0 {
				stack = append(stack, value)
			}
		case opEcho:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if err = host.Echo(value); err != nil {
				if handle(err) {
					continue
				}
				return err
			}
		case opIterInit:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			iterators[inst.a] = &iteratorState{entries: host.Entries(value), source: value}
		case opIterNext:
			iterator := iterators[inst.a]
			if iterator == nil || iterator.index >= len(iterator.entries) {
				pc = inst.target
				continue
			}
			entry := iterator.entries[iterator.index]
			iterator.index++
			iterator.key = entry.Key
			if inst.b >= 0 {
				locals[inst.b], initialized[inst.b] = entry.Key, true
			}
			locals[inst.c], initialized[inst.c] = entry.Value, true
		case opIterSet:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			iterator := iterators[inst.a]
			if iterator == nil {
				return fmt.Errorf("flatstack: pc %d: write-back to a closed iterator", pc)
			}
			if err = host.SetEntry(iterator.source, iterator.key, value); err != nil {
				if handle(err) {
					continue
				}
				return err
			}
		case opUnsetLocal:
			locals[inst.a], initialized[inst.a] = nil, false
		case opUnsetIndex:
			index, popErr := pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if err = host.UnsetIndex(base, index); err != nil {
				if handle(err) {
					continue
				}
				return err
			}
		case opCopyValue:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			stack = append(stack, model.CopyValue(value))
		case opIterClose:
			delete(iterators, inst.a)
		case opTryPush:
			handlers = append(handlers, errorHandler{
				target:     inst.target,
				local:      inst.a,
				start:      pc + 1,
				end:        inst.b,
				stackDepth: len(stack),
				frameDepth: len(callFrames),
			})
		case opTryPop:
			if len(handlers) == 0 {
				return fmt.Errorf("flatstack: pc %d: exception handler underflow", pc)
			}
			handlers = handlers[:len(handlers)-1]
		case opThrow:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			// A thrown throwable is already an error and propagates as
			// itself, so a catch clause binds the object rather than a
			// rendering of it. A bare value still renders.
			throwErr, ok := value.(error)
			if !ok {
				throwErr = fmt.Errorf("uncaught exception: %v", value)
			}
			if handle(throwErr) {
				continue
			}
			return throwErr
		case opReturn:
			retVal, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if len(callFrames) > 0 {
				lastFrame := callFrames[len(callFrames)-1]
				callFrames = callFrames[:len(callFrames)-1]
				pc = lastFrame.returnPC
				locals = lastFrame.locals
				initialized = lastFrame.initialized
				extras = lastFrame.extras
				if extras == nil {
					extras = map[string]any{}
				}
				stack = append(stack, retVal)
			} else {
				stack = append(stack, retVal)
				return nil
			}
		case opEnsureArray:
			value, popErr := pop()
			if popErr != nil {
				return popErr
			}
			if _, ok := value.(*model.Array); !ok && phpEmptyContainer(value) {
				value = host.Array(nil)
			}
			stack = append(stack, value)
		case opVivifyIndex:
			index, popErr := pop()
			if popErr != nil {
				return popErr
			}
			base, popErr := pop()
			if popErr != nil {
				return popErr
			}
			value := host.Index(base, index)
			if _, ok := value.(*model.Array); !ok && phpEmptyContainer(value) {
				value = host.Array(nil)
				if err = host.SetIndex(base, index, value, false, "="); err != nil {
					if handle(err) {
						continue
					}
					return err
				}
			}
			stack = append(stack, value)
		case opVivifyProperty:
			receiver, popErr := pop()
			if popErr != nil {
				return popErr
			}
			value := host.GetProperty(receiver, inst.name)
			if _, ok := value.(*model.Array); !ok && phpEmptyContainer(value) {
				value = host.Array(nil)
				if err = host.SetProperty(receiver, inst.name, value, "="); err != nil {
					if handle(err) {
						continue
					}
					return err
				}
			}
			stack = append(stack, value)
		case opInclude:
			pathValue, popErr := pop()
			if popErr != nil {
				return popErr
			}
			vars := make(map[string]any, len(program.localNames)+len(extras))
			for i, name := range program.localNames {
				if initialized[i] && (len(name) == 0 || name[0] != 0) {
					vars[name] = locals[i]
				}
			}
			for name, value := range extras {
				if _, ok := vars[name]; !ok {
					vars[name] = value
				}
			}
			includer, ok := host.(interface {
				Include(path any, keyword string, once bool, vars map[string]any) (any, map[string]any, error)
			})
			if !ok {
				return fmt.Errorf("flatstack: pc %d: host does not implement include", pc)
			}
			value, exported, includeErr := includer.Include(pathValue, inst.name, inst.a != 0, vars)
			if includeErr != nil {
				if handle(includeErr) {
					continue
				}
				return includeErr
			}
			applyNamedValues(program, locals, initialized, extras, exported)
			stack = append(stack, value)
		default:
			return fmt.Errorf("flatstack: pc %d: invalid opcode %d", pc, inst.op)
		}
		pc++
	}
	return nil
}

func bindHostLocals(host Host, program *Program, locals []any, initialized []bool, extras map[string]any) {
	binder, ok := host.(interface{ BindLocals(map[string]any) })
	if !ok {
		return
	}
	vars := make(map[string]any, len(program.localNames)+len(extras))
	for i, name := range program.localNames {
		if initialized[i] && (len(name) == 0 || name[0] != 0) {
			vars[name] = locals[i]
		}
	}
	for name, value := range extras {
		if _, ok := vars[name]; !ok {
			vars[name] = value
		}
	}
	binder.BindLocals(vars)
}

func applyHostLocals(host Host, program *Program, locals []any, initialized []bool, extras map[string]any) {
	taker, ok := host.(interface{ TakeLocals() map[string]any })
	if !ok {
		return
	}
	applyNamedValues(program, locals, initialized, extras, taker.TakeLocals())
}

func applyNamedValues(program *Program, locals []any, initialized []bool, extras map[string]any, names map[string]any) {
	if names == nil {
		return
	}
	for name, value := range names {
		if len(name) == 0 || name[0] == 0 {
			continue
		}
		found := false
		for i, localName := range program.localNames {
			if localName == name {
				locals[i], initialized[i] = value, true
				found = true
				break
			}
		}
		if !found && extras != nil {
			extras[name] = value
		}
	}
}

func lookupUserFunc(funcs map[string]userFuncDef, key string) (userFuncDef, bool) {
	if def, ok := funcs[key]; ok {
		return def, true
	}
	for name, def := range funcs {
		if strings.EqualFold(name, key) {
			return def, true
		}
	}
	return userFuncDef{}, false
}

func phpEmptyContainer(value any) bool {
	return value == nil
}
