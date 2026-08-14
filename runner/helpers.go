package runner

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// contextType is the reflect type of context.Context, used to detect callables
// that want the runtime context auto-injected as their first argument.
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// HostPanicError converts a panic raised by a registered Go constructor,
// function, or method into the runtime error path. PHP try/catch can therefore
// handle it like an error returned by the same host callable.
type HostPanicError struct {
	Callable string
	Value    any
}

func (e *HostPanicError) Error() string {
	return fmt.Sprintf("host panic in %s: %v", e.Callable, e.Value)
}

// wantsContext reports whether fn's first parameter is a context.Context.
func wantsContext(t reflect.Type) bool {
	return t.Kind() == reflect.Func && t.NumIn() > 0 && t.In(0) == contextType
}

// invokeWithScopeContext derives a context containing the active PHP frame so
// language helpers such as compact() can inspect caller-local variables and
// observability can identify the active source file.
func (rt *Runtime) invokeWithScopeContext(fn any, args []any, scope *Scope) (any, error) {
	if wantsContext(reflect.TypeOf(fn)) {
		full := make([]any, 0, len(args)+1)
		full = append(full, contextWithScope(contextWithEnv(rt.ctx, rt.Env), scope))
		full = append(full, args...)
		return invokeAny(fn, full)
	}
	return invokeAny(fn, args)
}

// This file implements the PHP-semantic helpers injected into every expr env.
// They encapsulate the behaviour expr-lang has no opinion about: PHP's ordered
// hybrid arrays, lenient index/property access, dynamic method dispatch (PHP
// objects vs. forwarded Go values), and object construction.
//
// Keeping these in Go (rather than emitting inline expr) means the transpiler
// output stays small and type-agnostic, and PHP semantics have one home.

// helperConcat implements PHP's `.` string concatenation with stringy coercion.
func helperConcat(a, b any) string {
	return phpString(a) + phpString(b)
}

// helperPair builds one array entry; key may be nil for list-style append.
func helperPair(key, val any) model.ArrayItemValue {
	return model.ArrayItemValue{Key: key, Val: val}
}

// helperArray constructs an ordered *model.Array from pairs, mirroring PHP's
// array() where unkeyed items take the next integer index.
func helperArray(items ...model.ArrayItemValue) *model.Array {
	arr := model.NewArray()
	for _, it := range items {
		if it.Key == nil {
			arr.Append(it.Val)
		} else {
			arr.Set(normalizeKey(it.Key), it.Val)
		}
	}
	return arr
}

// helperIndex implements `base[idx]` for *Array, Go maps and Go slices/arrays.
// Missing keys yield nil (PHP's forgiving access), not an error.
func helperIndex(base, idx any) any {
	switch b := base.(type) {
	case *model.Array:
		v, _ := b.Get(normalizeKey(idx))
		return v
	case string:
		// PHP string offset: $s[$i] returns the byte at position i as a string.
		i := toInt(idx)
		if i < 0 {
			i += int64(len(b))
		}
		if i < 0 || i >= int64(len(b)) {
			return ""
		}
		return string(b[i])
	case nil:
		return nil
	}
	rv := reflect.ValueOf(base)
	switch rv.Kind() {
	case reflect.Map:
		mv := rv.MapIndex(reflect.ValueOf(idx))
		if !mv.IsValid() {
			return nil
		}
		return mv.Interface()
	case reflect.Slice, reflect.Array:
		i := toInt(idx)
		if i < 0 || i >= int64(rv.Len()) {
			return nil
		}
		return rv.Index(int(i)).Interface()
	}
	return nil
}

// helperGet implements `base->name` / `base.name` property access against PHP
// objects (Props bag) and, by reflection, exported fields of Go structs. When a
// PHP object has no matching property but has a method by that name, it returns
// a callable bound to that object; this supports PHP idioms such as
// `call_user_func_array($this->query, $args)`.
func (rt *Runtime) helperGet(scope *Scope) func(base any, name string) any {
	return func(base any, name string) any {
		switch b := base.(type) {
		case *model.Object:
			if v, ok := b.Props[name]; ok {
				return v
			}
			if b.Class != nil {
				if decl, ok := b.Class.Methods[name]; ok {
					return func(args ...any) (any, error) { return rt.invokeMethod(b, decl, args, scope) }
				}
			}
			return nil
		case nil:
			return nil
		}
		rv := reflect.ValueOf(base)
		value := reflect.Indirect(rv)
		if value.Kind() == reflect.Struct {
			// Exact match first, then case-insensitive (PHP property access on a Go
			// struct: `$rec->value` resolves the exported field Value).
			if f := value.FieldByName(name); f.IsValid() {
				return f.Interface()
			}
			if f := value.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, name) }); f.IsValid() {
				return f.Interface()
			}
		}
		// A method read without parentheses is a bound callable, as in
		// `defer($db->close)` or `call_user_func_array($db->get, $args)`.
		// Return the runtime's uniform callable signature so indirect calls get
		// the same context injection and coercion as direct method calls.
		if m := rv.MethodByName(name); m.IsValid() {
			return rt.boundGoMethod(base, name, scope)
		}
		if m := methodByNameFold(rv, name); m.IsValid() {
			return rt.boundGoMethod(base, name, scope)
		}
		return nil
	}
}

func (rt *Runtime) boundGoMethod(base any, method string, scope *Scope) func(...any) (any, error) {
	return func(args ...any) (any, error) {
		callScope := scope
		if callScope == nil {
			callScope = NewScope()
		}
		return rt.callGoMethod(base, method, args, callScope)
	}
}

// adapt wraps any Go callable in the uniform func(...any) (any, error) signature
// used throughout the env. This serves two purposes:
//
//   - Type checking: the compile-time env (expr.Env) needs each function to have
//     a callable type, but a concrete signature (e.g. func(string) int) would
//     make expr reject dynamically-typed PHP arguments. A variadic-any signature
//     accepts anything.
//   - Runtime correctness: expr's compiled fast path asserts the env value has
//     the exact type seen at compile time. Using the same adapted value in both
//     the type env and the run env keeps those in sync.
//
// The wrapper performs PHP-ish argument coercion via reflection so shims can
// still be written with natural Go signatures.
func adapt(fn any) func(...any) (any, error) {
	return func(args ...any) (any, error) { return invokeAny(fn, args) }
}

// invokeAny calls fn (any Go callable) with args via reflection, coercing
// arguments to the declared parameter types where convertible.
func invokeAny(fn any, args []any) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = &HostPanicError{Callable: fmt.Sprintf("%T", fn), Value: recovered}
		}
	}()
	rv := reflect.ValueOf(fn)
	if rv.Kind() != reflect.Func {
		return nil, fmt.Errorf("not callable: %T", fn)
	}
	t := rv.Type()
	in := make([]reflect.Value, 0, len(args))
	for i, a := range args {
		in = append(in, coerceArg(a, paramType(t, i)))
	}
	// Pad missing non-variadic params with zero values (PHP tolerates this).
	for len(in) < t.NumIn() && !(t.IsVariadic() && len(in) >= t.NumIn()-1) {
		in = append(in, reflect.Zero(t.In(len(in))))
	}
	out := rv.Call(in)
	return firstReturn(out)
}

// paramType returns the declared parameter type at position i (handling variadic).
func paramType(t reflect.Type, i int) reflect.Type {
	n := t.NumIn()
	if n == 0 {
		return nil
	}
	if t.IsVariadic() && i >= n-1 {
		return t.In(n - 1).Elem()
	}
	if i < n {
		return t.In(i)
	}
	return nil
}

// coerceArg converts a value to the target parameter type where a cheap
// conversion makes it assignable; otherwise it is passed through.
func coerceArg(v any, want reflect.Type) reflect.Value {
	if want == nil {
		return reflect.ValueOf(v)
	}
	if v == nil {
		return reflect.Zero(want)
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(want) {
		return rv
	}
	if rv.Type().ConvertibleTo(want) {
		return rv.Convert(want)
	}
	return rv
}

// helperCall implements `base->method(args...)`. It dispatches PHP methods on
// model.Object instances back into the interpreter, and otherwise invokes a Go
// method on the value by reflection (the "invoke from values on the stack"
// capability the README relies on).
func (rt *Runtime) helperCall(scope *Scope) func(base any, method string, args ...any) (any, error) {
	return func(base any, method string, args ...any) (any, error) {
		if obj, ok := base.(*model.Object); ok && obj.Class != nil {
			if decl, ok := lookupPHPMethod(obj.Class, method); ok {
				return rt.invokeMethod(obj, decl, args, scope)
			}
		}
		return rt.callGoMethod(base, method, args, scope)
	}
}

func lookupPHPMethod(class *model.Class, method string) (*model.FuncDecl, bool) {
	if decl, ok := class.Methods[method]; ok {
		return decl, true
	}
	for name, decl := range class.Methods {
		if strings.EqualFold(name, method) {
			return decl, true
		}
	}
	return nil, false
}

// helperNew implements `new Class(args...)`: instantiate from the class table,
// apply field defaults, and run the same-named constructor method if present.
func (rt *Runtime) helperNew(scope *Scope) func(class string, args ...any) (any, error) {
	return func(class string, args ...any) (any, error) {
		defer rt.trace(scope, "new "+class)()

		class = strings.TrimPrefix(class, "\\")
		if !rt.hasClass(class) {
			if err := rt.autoload(class, scope); err != nil {
				return nil, err
			}
		}
		// A Go constructor takes precedence: `new Storage` becomes a native Go
		// value (storage, err := NewStorage(ctx)) with the context auto-injected
		// and any trailing error surfaced as a thrown error.
		if ctor, ok := rt.lookupConstructor(class); ok {
			v, err := rt.invokeWithScopeContext(ctor, args, scope)
			if err != nil {
				return nil, err
			}
			return v, nil
		}
		c, ok := rt.lookupClass(class)
		if !ok {
			return nil, fmt.Errorf("new: undefined class %q", class)
		}
		obj := model.NewObject(c)
		for _, f := range c.Fields {
			var def any
			if f.Default != nil {
				v, err := rt.Eval(f.Default, scope)
				if err != nil {
					return nil, err
				}
				def = v
			}
			obj.Props[f.Name] = def
		}
		// Constructor: prefer the modern __construct, fall back to the PHP4-style
		// method named after the class.
		ctor, ok := c.Methods["__construct"]
		if !ok {
			ctor, ok = c.Methods[class]
		}
		if !ok {
			// PHP4-style constructor named after the class's short name (the
			// part after the last namespace separator).
			if i := strings.LastIndexByte(class, '\\'); i >= 0 {
				ctor, ok = c.Methods[class[i+1:]]
			}
		}
		if ok {
			if _, err := rt.invokeMethod(obj, ctor, args, scope); err != nil {
				return nil, err
			}
		}
		return obj, nil
	}
}

// callGoMethod invokes an exported method on a Go value by reflection. Method
// names are matched case-insensitively (PHP method calls are case-insensitive,
// so `$obj->get()` resolves Go's exported Get). When the method's first
// parameter is a context.Context the runtime context is auto-injected, and
// arguments are coerced to the declared parameter types.
func (rt *Runtime) callGoMethod(base any, method string, args []any, scope *Scope) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = &HostPanicError{
				Callable: fmt.Sprintf("%T::%s", base, method),
				Value:    recovered,
			}
		}
	}()
	if base == nil {
		return nil, fmt.Errorf("call %s on nil", method)
	}
	rv := reflect.ValueOf(base)
	m := rv.MethodByName(method)
	if !m.IsValid() {
		m = methodByNameFold(rv, method)
	}
	if !m.IsValid() {
		return nil, fmt.Errorf("undefined method %T::%s", base, method)
	}
	mt := m.Type()
	if wantsContext(mt) {
		args = append([]any{contextWithScope(contextWithEnv(rt.ctx, rt.Env), scope)}, args...)
	}
	in := make([]reflect.Value, len(args))
	for i, a := range args {
		in[i] = coerceArg(a, paramType(mt, i))
	}
	out := m.Call(in)
	return firstReturn(out)
}

// methodByNameFold finds an exported method using PHP's case-insensitive
// semantics, then retries without underscores so snake_case PHP names resolve
// idiomatic Go names (for example get_all -> GetAll).
func methodByNameFold(rv reflect.Value, method string) reflect.Value {
	t := rv.Type()
	for i := 0; i < t.NumMethod(); i++ {
		if strings.EqualFold(t.Method(i).Name, method) {
			return rv.Method(i)
		}
	}
	method = strings.ReplaceAll(method, "_", "")
	for i := 0; i < t.NumMethod(); i++ {
		name := strings.ReplaceAll(t.Method(i).Name, "_", "")
		if strings.EqualFold(name, method) {
			return rv.Method(i)
		}
	}
	return reflect.Value{}
}

// firstReturn reduces a reflect Call result to (value, error) following Go
// conventions: a trailing error is surfaced, the first non-error value returned.
func firstReturn(out []reflect.Value) (any, error) {
	var result any
	for _, o := range out {
		if o.Type() == errorType {
			err, _ := o.Interface().(error)
			if err != nil {
				return result, err
			}
			continue
		}
		if result == nil {
			result = o.Interface()
		}
	}
	return result, nil
}
