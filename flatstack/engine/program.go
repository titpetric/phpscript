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
	opRef
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
	opRethrow
	opReturn
	opInclude
	opEnsureArray
	opVivifyIndex
	opVivifyProperty
	opClassConst
	opCast
	opClosure
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

// closureDef is one compiled anonymous function. Its body sits inline in the
// instruction stream, jumped over the way a function declaration's body is, and
// opClosure turns the definition into a callable value.
//
// The slot numbers are frame offsets, and the same number means the same name
// in every frame: Program.localNames is per program, not per function. That is
// what lets a capture be copied straight from the creating frame into the
// closure's own frame without a name lookup.
type closureDef struct {
	entryPC int
	// paramSlots holds one slot per declared parameter, in order. An argument
	// the caller did not pass leaves the slot null, which is what the
	// interpreter's bindParams does.
	paramSlots []int
	// captures holds the slots of the `use (...)` list. They are read where the
	// closure value is created, not where it is called, so the capture is the
	// snapshot PHP's by-value `use` describes.
	captures []int
	// thisSlot is the slot holding the receiver a closure written inside a
	// method carries away, or -1 for a `static function` and for one written
	// outside a class.
	thisSlot int
}

// catchClause is one compiled `catch (Type $var) { ... }`. declaredType is kept
// verbatim, including the `A|B` union form, because the host owns the matching
// rules; local is -1 when the clause binds no variable.
type catchClause struct {
	declaredType string
	local        int
	target       int
}

// Program is immutable bytecode compiled from a complete model.Program.
type Program struct {
	code       []instruction
	constants  []any
	localNames []string
	userFuncs  map[string]userFuncDef
	classes    []*model.Class
	// catchGroups holds the clause list of every compiled try, in source
	// order; opTryPush carries the index of its own group.
	catchGroups [][]catchClause
	// closures holds one entry per anonymous function in the program, in source
	// order; opClosure carries the index of its own definition.
	closures []closureDef
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
	// MatchCatch reports whether a catch clause declaring declaredType handles
	// err. The class hierarchy, the `A|B` union form and the rule that
	// `catch (Exception)` does not catch an engine error all live in the host,
	// so both backends select the same clause.
	MatchCatch(declaredType string, err error) bool
	// Throw turns a thrown value into the error it travels as. A built-in
	// throwable is an error already; an instance of a declared class is
	// wrapped so a clause can filter on the class it was declared as, and a
	// catch binding it gets the object back.
	Throw(value any) error
	// CatchValue returns what a catch clause binds for err, which is the object
	// for a thrown instance and the error itself for everything else.
	CatchValue(err error) any
	// ClassConst reads the constant name off class. The compiler has already
	// collapsed `self`, `static` and `parent` to the enclosing class, so the
	// name arrives concrete; `Class::class` still reaches the host, since it
	// resolves without the class being declared.
	ClassConst(class, name string) (any, error)
	// Cast applies a PHP type cast, spelled by its bare type name: "bool",
	// "int", "float", "string", "array".
	Cast(typ string, value any) any
	Binary(string, any, any) (any, error)
	Unary(string, any) (any, error)
	Truthy(any) bool
	Entries(any) []Entry
	Echo(any) error
}
