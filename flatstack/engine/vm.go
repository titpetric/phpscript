package engine

import (
	"errors"
	"fmt"

	"github.com/titpetric/phpscript/model"
)

type iteratorState struct {
	entries []Entry
	index   int
}

type errorHandler struct {
	target     int
	local      int
	start      int
	end        int
	stackDepth int
}

type callFrame struct {
	returnPC    int
	locals      []any
	initialized []bool
}

// Run executes a previously validated flat instruction stream.
func Run(program *Program, host Host) (err error) {
	if program == nil {
		return nil
	}
	pc := 0
	stack := make([]any, 0, 32)
	locals := make([]any, len(program.localNames))
	initialized := make([]bool, len(program.localNames))
	iterators := make(map[int]*iteratorState)
	var handlers []errorHandler
	var callFrames []callFrame
	defer func() {
		clear(stack)
		clear(locals)
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
		last := len(handlers) - 1
		handler := handlers[last]
		handlers = handlers[:last]
		if handler.stackDepth < len(stack) {
			clear(stack[handler.stackDepth:])
			stack = stack[:handler.stackDepth]
		}
		for errors.Unwrap(runErr) != nil {
			runErr = errors.Unwrap(runErr)
		}
		if handler.local >= 0 {
			locals[handler.local], initialized[handler.local] = runErr, true
		}
		pc = handler.target
		return true
	}

	for pc < len(program.code) {
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
		case opCall, opConstruct:
			arguments, argErr := args(inst.a)
			if argErr != nil {
				return argErr
			}
			var value any
			if inst.op == opCall {
				if def, ok := program.userFuncs[inst.name]; ok {
					callFrames = append(callFrames, callFrame{
						returnPC:    pc,
						locals:      locals,
						initialized: initialized,
					})
					locals = make([]any, len(program.localNames))
					initialized = make([]bool, len(program.localNames))
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
			} else {
				value, err = host.Construct(inst.name, arguments)
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
			receiver, popErr := pop()
			if popErr != nil {
				return popErr
			}
			value, callErr := host.CallMethod(receiver, inst.name, arguments)
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
			iterators[inst.a] = &iteratorState{entries: host.Entries(value)}
		case opIterNext:
			iterator := iterators[inst.a]
			if iterator == nil || iterator.index >= len(iterator.entries) {
				pc = inst.target
				continue
			}
			entry := iterator.entries[iterator.index]
			iterator.index++
			if inst.b >= 0 {
				locals[inst.b], initialized[inst.b] = entry.Key, true
			}
			locals[inst.c], initialized[inst.c] = entry.Value, true
		case opIterClose:
			delete(iterators, inst.a)
		case opTryPush:
			handlers = append(handlers, errorHandler{
				target:     inst.target,
				local:      inst.a,
				start:      pc + 1,
				end:        inst.b,
				stackDepth: len(stack),
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
			throwErr := fmt.Errorf("uncaught exception: %v", value)
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
				stack = append(stack, retVal)
			} else {
				stack = append(stack, retVal)
				return nil
			}
		default:
			return fmt.Errorf("flatstack: pc %d: invalid opcode %d", pc, inst.op)
		}
		pc++
	}
	return nil
}
