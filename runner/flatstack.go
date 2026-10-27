package runner

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	flatvm "github.com/titpetric/phpscript/flatstack/engine"
	"github.com/titpetric/phpscript/model"
)

func (rt *Runtime) runFlat(ast *model.Program) (bool, error) {
	if rt.errorHandler != nil {
		// The interpreter can recover per statement through OnError. The flat
		// backend returns the first execution error.
		return false, nil
	}
	program, ok := rt.exprCache.getFlat(ast)
	if !ok {
		var err error
		program, err = flatvm.Compile(ast)
		if err != nil {
			// A compile error normally means the program uses something the
			// bytecode subset does not cover yet, and the interpreter runs it
			// instead. A violated interface contract differs: the verdict is
			// on the program, and the interpreter would reach the same one,
			// and this path throws.
			var contract *model.InterfaceContractError
			if errors.As(err, &contract) {
				return true, NewRuntimeException(err.Error(), 0)
			}
			return false, nil
		}
		rt.exprCache.setFlat(ast, program)
	}
	if err := rt.hoistOnce(ast, rt.entrypoint); err != nil {
		// A redeclaration is a verdict on the program, like a violated
		// interface contract above: the interpreter would reach it too and
		// falling back would only run the same hoist a second time, over a
		// table the first pass already wrote into, and report the wrong
		// declaration as the duplicate.
		var redeclared *RedeclareError
		if errors.As(err, &redeclared) {
			return true, NewRuntimeException(err.Error(), 0)
		}
		return false, nil
	}
	if rt.hostFlat == nil {
		// One host per runtime: the only per-run state it carries is the
		// frame handle, which the VM binds and restores itself.
		rt.hostFlat = &flatHost{runtime: rt}
	}
	return true, flatvm.Run(rt.hostFlat, program)
}

type flatHost struct {
	runtime *Runtime
	// frame is the engine's handle on the frame in flight, bound once per
	// run. It replaces the map copied in and out around every call: the host
	// snapshots only when a callee actually needs the scope.
	frame flatvm.FrameLocals
}

func (h *flatHost) BindFrame(frame flatvm.FrameLocals) { h.frame = frame }

func (h *flatHost) TakeFrame() flatvm.FrameLocals { return h.frame }

// SetGlobal claims whole-variable stores to superglobal names: those bind one
// shared array per request, which interpreted and bytecode frames alike read
// back through Lookup. Any other name stays with the storing frame.
func (h flatHost) SetGlobal(name string, value any) bool {
	return h.runtime.auto.Set(name, value)
}

func (h flatHost) GetProperty(receiver any, name string) any {
	return h.runtime.helperGet(&scopeRef{})(receiver, name)
}

// SetProperty writes an object property, the same way the interpreter does:
// a PHP object carries the value in its property map, and a Go binding assigns
// the struct field the name resolves to and `$db->is_readonly = true` sets
// IsReadonly.
func (h flatHost) SetProperty(receiver any, name string, value any, op string) error {
	if object, ok := receiver.(*model.Object); ok {
		next, err := applyAssignOp(op, object.Props[name], value)
		if err != nil {
			return err
		}
		object.SetProp(name, next)
		return nil
	}
	return assignGoField(receiver, name, func(current any) (any, error) {
		return applyAssignOp(op, current, value)
	})
}

// Echo writes through the runtime output stack and not to the base writer,
// and ob_start captures bytecode output the way it captures interpreted output.
func (h flatHost) Echo(value any) error {
	_, err := io.WriteString(h.runtime.Output(), phpString(value))
	return err
}

// Globals returns the table runInterpreted seeds its global scope from and the
// bytecode engine's top-level frame starts with the same names. Lookup, which
// every frame reads, answers none of them: a global belongs to the global
// scope, and resolveVar does not reach one from a function either.
func (h flatHost) Globals() map[string]any { return h.runtime.globals }

// Lookup is resolveVar's tail: the superglobals, which every frame sees, and
// then the constant table. A frame's own names are the engine's slots, and
// Globals seeds a global into the top-level frame.
func (h flatHost) Lookup(name string) any {
	if value, ok := h.runtime.resolveAuto(name); ok {
		return value
	}
	return h.runtime.constants[name]
}

// Constant resolves a bare name: whatever the host knows under it, then the
// constant table, and an Error when nothing does. PHP 8 raises the same for
// the same expression, and an unset variable of that spelling stays null,
// and this is separate from Lookup.
func (h flatHost) Constant(name string) (any, error) {
	if value, ok := h.runtime.resolveAuto(name); ok {
		return value, nil
	}
	if value, ok := h.runtime.constants[name]; ok {
		return value, nil
	}
	return nil, &UndefinedConstantError{Name: name}
}

// SetConstant is the write side of Constant: a top-level `const` entry lands
// in the same table define() writes through Runtime.SetConst.
func (h flatHost) SetConstant(name string, value any) {
	h.runtime.SetConst(name, value)
}

func (h flatHost) Array(items []model.ArrayItemValue) any { return helperArray(items...) }

func (h flatHost) Index(base, index any) any { return helperIndex(base, index) }

func (h flatHost) Truthy(value any) bool { return phpTruthy(value) }

// SetEntry implements the by-reference foreach write-back. Only a *model.Array
// is script-owned storage; a native Go collection a binding returned belongs to
// the host and this drops the write and reports nothing. Runtime.execForeach
// takes the same view.
func (h flatHost) SetEntry(container, key, value any) error {
	array, ok := container.(*model.Array)
	if !ok {
		return nil
	}
	array.Set(normalizeKey(key), value)
	return nil
}

// UnsetIndex implements unset($a[$k]) for the bytecode engine.
func (h flatHost) UnsetIndex(container, key any) (any, error) {
	array, ok := container.(*model.Array)
	if !ok {
		replacement, _ := unsetGoIndex(container, key)
		return replacement, nil
	}
	array.Delete(normalizeKey(key))
	return nil, nil
}

func (h flatHost) SetIndex(base, key, value any, appendValue bool, op string) error {
	array, ok := base.(*model.Array)
	if !ok {
		// Native Go collections are writable where Go itself allows it, the
		// same as in the interpreter: only `$a[] =` needs an *Array, since a
		// slice cannot grow through the interface holding it.
		if appendValue {
			return fmt.Errorf("assign: cannot append to %T; a binding whose result is appended to must return *model.Array", base)
		}
		return assignGoIndex(base, key, func(current any) (any, error) {
			return applyAssignOp(op, current, value)
		})
	}
	if appendValue {
		array.Append(value)
		return nil
	}
	key = normalizeKey(key)
	if op != "" && op != "=" {
		current, _ := array.Get(key)
		binaryOp := strings.TrimSuffix(op, "=")
		var err error
		value, err = h.Binary(binaryOp, current, value)
		if err != nil {
			return err
		}
	}
	array.Set(key, value)
	return nil
}

// MatchCatch answers the bytecode engine's clause selection with the rules the
// interpreter uses and `catch (Exception $e)` declines a TypeError on both
// backends and not on one of them.
func (h flatHost) MatchCatch(declaredType string, err error) bool {
	return matchCatchType(declaredType, err)
}

// ClassConst reads a class constant, reusing the interpreter's resolution so
// `Class::class`, autoloading and the per-class cache behave identically on
// both backends. The compiler already collapsed self/static/parent, but the
// scope still carries the class: a constant's default expression runs here,
// and it may name the class again.
func (h flatHost) ClassConst(class, name string) (any, error) {
	scope := h.runtime.newScope()
	scope.Set("__class__", h.runtime.boxed(class))
	return h.runtime.helperClassConst(&scopeRef{scope: scope})(class, name)
}

func (h flatHost) Cast(typ string, value any) any { return helperCast(typ, value) }

// Throw turns a thrown value into the error it travels as, the same way the
// interpreter's throw statement does.
func (h flatHost) Throw(value any) error {
	if thrown, ok := value.(error); ok {
		return thrown
	}
	if obj, ok := value.(*model.Object); ok {
		return newObjectError(obj)
	}
	return fmt.Errorf("uncaught exception: %s", phpString(value))
}

// CatchValue returns what a catch clause binds for err.
func (h flatHost) CatchValue(err error) any { return catchValue(err) }

func (h flatHost) Binary(op string, left, right any) (any, error) {
	switch op {
	case ".":
		return helperConcat(left, right), nil
	case "+", "-", "*", "/", "%", "**":
		return phpArith(op, left, right), nil
	case "&", "|", "^", "<<", ">>":
		return phpBitwise(op, left, right)
	case "instanceof":
		return phpInstanceOf(left, right), nil
	case "==", "!=", "===", "!==", "<", "<=", ">", ">=":
		return phpCompare(op, left, right), nil
	case "&&":
		return phpTruthy(left) && phpTruthy(right), nil
	case "||":
		return phpTruthy(left) || phpTruthy(right), nil
	default:
		return nil, fmt.Errorf("unsupported binary operator %q", op)
	}
}

func (h flatHost) Unary(op string, value any) (any, error) {
	switch op {
	case "!":
		return !phpTruthy(value), nil
	case "+":
		return phpArith("+", int64(0), value), nil
	case "-":
		return phpNegate(value), nil
	case "~":
		return phpBitNot(value), nil
	default:
		return nil, fmt.Errorf("unsupported unary operator %q", op)
	}
}

func (h flatHost) Entries(value any) []flatvm.Entry {
	var entries []flatvm.Entry
	if array, ok := value.(*model.Array); ok {
		array.Range(func(key, value any) bool {
			entries = append(entries, flatvm.Entry{Key: key, Value: value})
			return true
		})
		return entries
	}
	if object, ok := value.(*model.Object); ok {
		object.Range(func(name string, value any) bool {
			entries = append(entries, flatvm.Entry{Key: name, Value: value})
			return true
		})
		return entries
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			entries = append(entries, flatvm.Entry{Key: int64(i), Value: rv.Index(i).Interface()})
		}
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			entries = append(entries, flatvm.Entry{Key: key.Interface(), Value: rv.MapIndex(key).Interface()})
		}
	}
	return entries
}

func (h flatHost) RegisterClass(class *model.Class) {
	for _, method := range class.Methods {
		if method != nil && method.Filename == "" {
			method.Filename = h.runtime.entrypoint
		}
	}
	h.runtime.RegisterClass(class)
}

func (h flatHost) Include(path any, keyword string, once bool, vars map[string]any) (any, map[string]any, error) {
	scope := h.runtime.newScope()
	for name, value := range vars {
		scope.Set(name, value)
	}
	// The *_once dedupe lives in includeFile and both engines answer it on the
	// resolved path and neither keeps its own scan over spellings.
	result, err := h.runtime.includeFile(phpString(path), once, scope)
	if err != nil {
		return nil, nil, err
	}
	exported := make(map[string]any, len(scope.vars))
	for name, value := range scope.vars {
		if name == "__FILE__" || name == "__DIR__" {
			continue
		}
		exported[name] = value
	}
	_ = keyword
	return result, exported, nil
}

// MemoryCheckInterval reports how often the VM should poll the memory limit;
// zero when the options name no limit.
func (h flatHost) MemoryCheckInterval() int {
	if h.runtime.opts.MemoryLimit <= 0 {
		return 0
	}
	return memCheckInstructions
}

func (h flatHost) PushLiveWalker(walk func(yield func(any))) {
	h.runtime.vmWalkers = append(h.runtime.vmWalkers, walk)
}

func (h flatHost) PopLiveWalker() {
	h.runtime.vmWalkers = h.runtime.vmWalkers[:len(h.runtime.vmWalkers)-1]
}

// CheckDeadline answers the engine's per-run deadline check; see
// runner/runtime_deadline.go for what the two watch flags mean.
func (h flatHost) CheckDeadline() error {
	if !h.runtime.watchingDeadline() {
		return nil
	}
	return h.runtime.checkDeadline()
}

func (h flatHost) CheckMemory() error {
	return h.runtime.checkMemory()
}

// UnsetProperty removes a named property, PHP's unset($obj->prop). Only a
// PHP object carries removable storage, and this passes over anything else,
// the leniency execUnset applies.
func (h flatHost) UnsetProperty(receiver any, name string) error {
	if object, ok := receiver.(*model.Object); ok {
		object.DeleteProp(name)
	}
	return nil
}

// StaticVarBag returns the persistent storage of one `static $x` statement,
// the same per-node table the interpreter's named functions use and the
// counts agree even when the two backends run the same program in turn.
func (h flatHost) StaticVarBag(node *model.StaticVar) (map[string]any, bool) {
	bag, seeded := h.runtime.funcStatics[node]
	if !seeded {
		bag = map[string]any{}
		h.runtime.funcStatics[node] = bag
	}
	return bag, seeded
}
