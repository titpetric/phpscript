// Package engine compiles model AST programs into flat bytecode and executes
// them against a host-provided PHP semantics bridge.
package engine

import "github.com/titpetric/phpscript/model"

type opcode uint8

const (
	opPushConst opcode = iota
	opPop
	opDup
	opLoad
	opStore
	opArray
	opIndex
	opSetIndex
	opIncDecLocal
	opIncDecIndex
	opBinary
	opUnary
	opTruthy
	opJump
	opJumpFalse
	opJumpTrue
	opCall
	opConstruct
	opCallMethod
	opGetProperty
	opSetProperty
	opEcho
	opIterInit
	opIterNext
	opIterSet
	opIterClose
	opCopyValue
	opUnsetLocal
	opUnsetIndex
	opTryPush
	opTryPop
	opThrow
	opReturn
)

type instruction struct {
	op     opcode
	a      int
	b      int
	c      int
	target int
	name   string
	extra  string
}

type userFuncDef struct {
	entryPC int
	params  []string
}

// Program is immutable bytecode compiled from a complete model.Program.
type Program struct {
	code       []instruction
	constants  []any
	localNames []string
	userFuncs  map[string]userFuncDef
}

// Entry is one key/value pair produced for foreach.
type Entry struct {
	Key   any
	Value any
}

// Host owns PHP value semantics and the Go API bridge. The engine itself owns
// only compilation, operand/local storage, jumps, and iteration state.
type Host interface {
	Construct(string, []any) (any, error)
	CallMethod(any, string, []any) (any, error)
	Call(string, string, []any) (any, error)
	GetProperty(any, string) any
	SetProperty(any, string, any, string) error
	Lookup(string) any
	Array([]model.ArrayItemValue) any
	Index(any, any) any
	SetIndex(any, any, any, bool, string) error
	// SetEntry writes value into container at key when container is a value the
	// script owns. A collection a binding returned belongs to the host, so it is
	// left alone rather than reported as an error, matching what a by-reference
	// foreach over one does in the interpreter.
	SetEntry(container, key, value any) error
	// UnsetIndex removes key from container, PHP's unset($a[$k]). Removing a
	// key that is not there is not an error.
	UnsetIndex(container, key any) error
	Binary(string, any, any) (any, error)
	Unary(string, any) (any, error)
	Truthy(any) bool
	Entries(any) []Entry
	Echo(any) error
}
