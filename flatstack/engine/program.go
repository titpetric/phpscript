// Package engine compiles model AST programs into flat bytecode and executes
// them against a host-provided PHP semantics bridge.
package engine

import (
	"github.com/titpetric/phpscript/model"
)

type opcode uint8

const (
	opPushConst opcode = iota
	opPop
	opDup
	opLoad
	// opLoadConst reads a bare name. It is separate from opLoad because the
	// two answer differently when nothing defines the name: an unset variable
	// is null, an undefined constant is an Error.
	opLoadConst
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
	// opDefineConst pops the value and declares it under the name a top-level
	// `const` statement wrote, through the same host table define() uses.
	opDefineConst
	// opDefer pops the callable and registers it on the frame in flight; the
	// VM runs the registrations LIFO when the frame returns.
	opDefer
	// opInvoke calls a callable held in a value: a args, then the callee
	// beneath them. A string callee naming a compiled function takes a VM
	// frame; everything else goes to the host's InvokeValue.
	opInvoke
	// opIncDecProp is `$obj->prop++` and friends: pops the receiver, reads the
	// property, steps it, writes it back. b selects postfix, name is the
	// property, extra the operator.
	opIncDecProp
	// opUnsetProp pops the receiver and removes the named property through the
	// host, PHP's unset($obj->prop).
	opUnsetProp
	// opCompactInit pushes the empty map[string]any that compact() fills. A
	// native map rather than a script array, which is what the binding
	// returns and what the allocation rules prefer.
	opCompactInit
	// opCompactEntry writes one name into the map on top of the stack, which
	// it leaves there: a is the local's slot and name is the key. A slot
	// holding nothing is skipped, which is how compact() omits a name that is
	// not set; a slot holding null is not, because null is a value.
	opCompactEntry
	// opCompactDynamic is opCompactEntry for a name the compiler could not
	// read off the source: it pops the name and looks the slot up in the
	// table the program carries. compact($which) and compact(name()) take
	// this path.
	opCompactDynamic
	// opCallStatic is `Class::method(args...)`: a args, name the class
	// (contextual names already collapsed), extra the method. b=1 is the
	// `Class::$m(...)` form: the method name value sits beneath the args and
	// extra is empty.
	opCallStatic
	// opStaticProp reads `Class::$name`: name the class, extra the property.
	opStaticProp
	// opSetStaticProp writes `Class::$name`: pops the value, a indexes the
	// constant pool for the assignment operator spelling, b keeps the value on
	// the stack for the expression form.
	opSetStaticProp
	// opStaticSeeded pushes whether the function-static bag a indexes (a
	// *model.StaticVar in the constant pool) already holds values from an
	// earlier call, which is what decides whether the initializers run.
	opStaticSeeded
	// opStaticLoad and opStaticStore read and write one name in a
	// function-static bag: a indexes the *model.StaticVar node, name is the
	// variable. The bag is live shared storage, so a recursive call observes
	// the writes of the frame above it. opStaticStore mirrors opStore's b
	// (keep) and compound-operator name handling through extra.
	opStaticLoad
	opStaticStore
	// opIncDecStatic is opIncDecLocal against a function-static bag entry.
	opIncDecStatic
	// opInitialized pushes whether slot a holds a value, the test a parameter
	// default's prologue jumps on.
	opInitialized
	// Register-form binaries, written by the fusion pass (fuse.go): operands
	// come from slots (L) or the constant pool (C) instead of the operand
	// stack, and target selects push (0) or a plain store into slot
	// target-1. opBinTC takes its left operand off the stack.
	opBinLL
	opBinLC
	opBinTC
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

// Binary operator classes, resolved at compile time into opBinary's b field,
// the way runner/expr captures the operator when it compiles a closure. The
// VM inlines the both-int64 (and both-string, for concat) cases through the
// phpval helpers phpArith itself uses; every other operand shape falls
// through to host.Binary with the operator name, so semantics have one home.
// binNone is zero so an unclassified emit keeps its host dispatch.
const (
	binNone = iota
	binAdd
	binSub
	binMul
	binDiv
	binMod
	binLt
	binLe
	binGt
	binGe
	binEq
	binNe
	binIdent
	binNotIdent
	binConcat
)

// binOpClass classifies an operator spelling, binNone when the VM has no
// inline case for it.
func binOpClass(op string) int {
	switch op {
	case "+":
		return binAdd
	case "-":
		return binSub
	case "*":
		return binMul
	case "/":
		return binDiv
	case "%":
		return binMod
	case "<":
		return binLt
	case "<=":
		return binLe
	case ">":
		return binGt
	case ">=":
		return binGe
	case "==":
		return binEq
	case "!=":
		return binNe
	case "===":
		return binIdent
	case "!==":
		return binNotIdent
	case ".":
		return binConcat
	}
	return binNone
}

type userFuncDef struct {
	entryPC int
	// paramSlots holds one slot per declared parameter, in order, resolved at
	// compile time the way closureDef's are. An argument the caller did not
	// pass leaves the slot uninitialized.
	paramSlots []int
	// variadicSlot is the slot of a trailing `...$rest` parameter, -1 without
	// one. It is not in paramSlots: the caller binds it to an array of the
	// leftover arguments, empty when the caller stops short of it.
	variadicSlot int
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
	// nameSlots is the inverse of localNames, built once by the compiler so
	// run-time name resolution is a map hit instead of a linear scan.
	nameSlots map[string]int
	userFuncs map[string]userFuncDef
	// userFuncsFold indexes userFuncs by lowercased name; PHP function and
	// class names are case-insensitive, so a fold collision is already a
	// redeclaration error before this map is built.
	userFuncsFold map[string]userFuncDef
	classes       []*model.Class
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
	// SetGlobal offers a whole-variable store to the host before the frame
	// keeps it. A host claims names with cross-frame semantics — PHP's
	// superglobals — by returning true; the engine then leaves the frame slot
	// cold, so later reads keep resolving through Lookup.
	SetGlobal(string, any) bool
	// Constant resolves a bare name. The error is what an undefined one
	// raises, which Lookup has no way to report.
	Constant(string) (any, error)
	// SetConstant declares a constant, the write side of Constant. A
	// top-level `const` entry goes through it so both spellings of a global
	// constant share one table.
	SetConstant(string, any)
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
	//
	// It answers the container to store back, or nil when the removal was in
	// place. A native slice cannot hold a hole and cannot shrink through the
	// interface value holding it, so removing from one answers a shorter
	// slice for the caller to assign.
	UnsetIndex(container, key any) (replacement any, err error)
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
	// InvokeCallable runs a deferred callable value on the host bridge.
	InvokeCallable(any) error
}

// The optional host capabilities, discovered by type assertion the way
// MemoryHost and the include hook are. A Host built before these constructs
// compiled keeps building; a program that reaches one against a host without
// it reports the missing capability instead of misbehaving.

// invokeHost calls a callable held in a value, `$fn(...)`, resolving every
// PHP callable spelling.
type invokeHost interface {
	InvokeValue(callee any, args []any) (any, error)
}

// unsetPropHost removes a named property, PHP's unset($obj->prop). Removing
// one that is not there, or from a value that has none, is not an error.
type unsetPropHost interface {
	UnsetProperty(receiver any, name string) error
}

// staticCallHost dispatches `Class::method(args...)`. method arrives as a
// value because the `Class::$m(...)` spelling carries it at run time.
type staticCallHost interface {
	CallStatic(class string, method any, args []any) (any, error)
}

// staticPropHost reads and writes `Class::$name`, the storage the class owns.
// SetStaticProp applies op, so a compound assignment reads and writes under
// the host's rules.
type staticPropHost interface {
	GetStaticProp(class, name string) (any, error)
	SetStaticProp(class, name string, value any, op string) error
}

// staticVarHost returns the persistent bag of one `static $x` statement and
// whether an earlier call already seeded it. The bag is live shared storage:
// the VM reads and writes it directly, so a recursive call observes the
// frame above it.
type staticVarHost interface {
	StaticVarBag(node *model.StaticVar) (map[string]any, bool)
}
