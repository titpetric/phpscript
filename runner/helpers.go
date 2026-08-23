package runner

import (
	"context"
	"errors"
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
		args = full
	}
	result, err := invokeAny(fn, args)
	if err == nil && rt.opts.MemoryLimit > 0 {
		// Burst guard: a single host call can allocate far more than the
		// per-statement checkpoint interval sees (str_repeat, file reads).
		// The shallow estimate only decides when to walk early; the walk is
		// the truth and resets the pending counter.
		if rt.memPending += EstimateValueSize(result); rt.memPending > rt.opts.MemoryLimit.Bytes()/8 {
			if memErr := rt.checkMemory(); memErr != nil {
				return nil, memErr
			}
		}
	}
	return result, err
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
	arr := model.NewArraySize(len(items))
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
func (rt *Runtime) helperGet(ref *scopeRef) func(base any, name string) any {
	return func(base any, name string) any {
		// A bound callable produced here can outlive the expression that read it,
		// so the scope is captured by value rather than through the reference.
		scope := ref.scope
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
			if f := fieldByNameFold(value, name); f.IsValid() && f.CanInterface() {
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
			callScope = rt.newScope()
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

// ArgumentCountError reports a call that passed more arguments than the
// callable declares. PHP raises the same condition as ArgumentCountError; this
// runtime registers that name alongside every other throwable class, and a
// returned error is catchable whichever of them a script names.
type ArgumentCountError struct {
	Name string
	Want int
	Got  int
}

func (e *ArgumentCountError) Error() string {
	name := e.Name
	if name == "" {
		name = "call"
	}
	return fmt.Sprintf("%s() expects at most %s, %d given", name, plural(e.Want, "argument"), e.Got)
}

// TypeError reports an argument that cannot be converted to the type the
// callable's parameter declares. PHP raises TypeError for the same call, and
// the Go type name is what a script sees as the class, so this is named for
// the PHP class rather than for what it holds: `catch (TypeError $e)` has to
// match it, and `catch (Exception $e)` has to not.
type TypeError struct {
	Name     string
	Position int
	Want     string
	Got      string
}

// Error renders the message PHP's TypeError carries for the same call.
func (e *TypeError) Error() string {
	name := e.Name
	if name == "" {
		name = "call"
	}
	// PHP also names the parameter ("Argument #3 ($limit)"). A Go binding's
	// parameter names are not recoverable through reflect, so the position
	// stands alone.
	return fmt.Sprintf("%s(): Argument #%d must be of type %s, %s given", name, e.Position, e.Want, e.Got)
}

// nameCallError fills in the PHP name of an error raised while building a call.
// invokeAny works from the Go signature alone and has no name to report; the
// name a script typed is known only at the dispatch site.
func nameCallError(err error, name string) error {
	var count *ArgumentCountError
	if errors.As(err, &count) && count.Name == "" {
		count.Name = name
		return err
	}
	var mismatch *TypeError
	if errors.As(err, &mismatch) && mismatch.Name == "" {
		mismatch.Name = name
	}
	return err
}

// phpParamTypeName spells a declared Go parameter type the way PHP names the
// type in a TypeError ("must be of type int"). It describes a *parameter*, so
// it is deliberately separate from phpDebugType, which describes a value, and
// from gettype's legacy names (integer/double/boolean) in stdlib: neither table
// may be folded into the other.
func phpParamTypeName(t reflect.Type) string {
	if t == nil {
		return "mixed"
	}
	if t == reflect.TypeOf((*model.Array)(nil)) {
		return "array"
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Slice, reflect.Map, reflect.Array:
		return "array"
	case reflect.Func:
		return "callable"
	case reflect.Interface:
		return "mixed"
	}
	return t.String()
}

// phpDebugType names a value the way PHP's get_debug_type() does ("array
// given"). It describes a *value*, so it is deliberately separate from
// phpParamTypeName, which describes a declared parameter type, and from
// gettype's legacy names (integer/double/boolean) in stdlib: neither table may
// be folded into the other.
func phpDebugType(v any) string {
	switch value := v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case string:
		return "string"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "int"
	case float32, float64:
		return "float"
	case *model.Array:
		return "array"
	case *model.Object:
		if value.Class != nil {
			return value.Class.Name
		}
		return "object"
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return "array"
	case reflect.Func:
		return "Closure"
	}
	return rv.Type().String()
}

// buildArgs coerces args to the declared parameter types of t, padding absent
// trailing parameters with zero values: a Go binding spells PHP's optional
// parameters as extra ones, so a short call is ordinary. A call with more
// arguments than t declares is refused, because reflect.Value.Call panics on
// it and PHP refuses the same call to an internal function. An argument that
// converts to no declared type is refused for the same reason: reflect.Value
// .Call panics on it, and PHP raises a catchable TypeError.
func buildArgs(t reflect.Type, args []any, name string) ([]reflect.Value, error) {
	if !t.IsVariadic() && len(args) > t.NumIn() {
		return nil, &ArgumentCountError{Name: name, Want: t.NumIn(), Got: len(args)}
	}
	in := make([]reflect.Value, 0, len(args))
	// The runtime context, when a binding asks for one, is injected ahead of
	// the script's arguments and so does not count towards the PHP position.
	offset := 1
	if wantsContext(t) {
		offset = 0
	}
	for i, a := range args {
		want := paramType(t, i)
		v, ok := coerceArg(a, want)
		if !ok {
			return nil, &TypeError{
				Name:     name,
				Position: i + offset,
				Want:     phpParamTypeName(want),
				Got:      phpDebugType(a),
			}
		}
		in = append(in, v)
	}
	for len(in) < t.NumIn() && !(t.IsVariadic() && len(in) >= t.NumIn()-1) {
		in = append(in, reflect.Zero(t.In(len(in))))
	}
	return in, nil
}

// plural renders a count with its noun, as PHP spells an argument count error.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// argAt returns the i-th argument, or nil when the script passed fewer
// arguments than the binding declares. PHP tolerates the short call, and the
// reflect path pads the same way with zero values.
func argAt(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return nil
}

// invokeFast calls the binding directly when its signature is one of the
// shapes stdlib registers most, skipping reflect.Value.Call and the []reflect
// .Value slice it needs. The final return reports whether the signature was
// recognised; a false sends the caller to the reflect path.
//
// Callers must keep this behind invokeAny's panic boundary: a binding that
// panics here has to surface as a HostPanicError just as it does under
// reflection, or it unwinds the VM instead of becoming a PHP exception.
func invokeFast(fn any, args []any) (any, error, bool) {
	switch f := fn.(type) {
	case func(...any) (any, error):
		v, err := f(args...)
		return v, err, true
	case func(any) any:
		return f(argAt(args, 0)), nil, true
	case func(any) bool:
		return f(argAt(args, 0)), nil, true
	case func(any) string:
		return f(argAt(args, 0)), nil, true
	case func(any, any) string:
		return f(argAt(args, 0), argAt(args, 1)), nil, true
	case func(any, any) any:
		return f(argAt(args, 0), argAt(args, 1)), nil, true
	case func(string) string:
		return f(phpString(argAt(args, 0))), nil, true
	case func() string:
		return f(), nil, true
	case func() any:
		return f(), nil, true
	case func(...any) any:
		return f(args...), nil, true
	case func(...any) bool:
		return f(args...), nil, true
	}
	return nil, nil, false
}

// invokeAny calls fn (any Go callable) with args, coercing arguments to the
// declared parameter types where convertible. Common signatures are dispatched
// directly by invokeFast; the rest go through reflection.
func invokeAny(fn any, args []any) (result any, err error) {
	// The boundary covers both dispatch paths. Registered code is host code,
	// and a panic crossing into the VM has to arrive as a catchable PHP
	// exception rather than unwinding the interpreter.
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = &HostPanicError{Callable: fmt.Sprintf("%T", fn), Value: recovered}
		}
	}()
	ft := reflect.TypeOf(fn)
	if ft == nil || ft.Kind() != reflect.Func {
		return nil, fmt.Errorf("not callable: %T", fn)
	}
	// PHP's internal functions reject a call passing more arguments than they
	// declare, and reflect.Value.Call panics on the same call. Report it as an
	// ordinary error, before either dispatch path, so the two agree and a
	// script can catch it. Too few arguments stay legal: a Go binding spells
	// PHP's optional parameters as extra ones, and they are zero-padded below.
	if !ft.IsVariadic() && len(args) > ft.NumIn() {
		return nil, &ArgumentCountError{Want: ft.NumIn(), Got: len(args)}
	}
	if fast, fastErr, ok := invokeFast(fn, args); ok {
		return fast, fastErr
	}
	rv := reflect.ValueOf(fn)
	in, err := buildArgs(rv.Type(), args, "")
	if err != nil {
		return nil, err
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
// conversion makes it assignable. The final return reports whether it did: a
// false means the value cannot be passed, and the caller turns that into a PHP
// TypeError rather than letting reflect.Value.Call panic on it.
func coerceArg(v any, want reflect.Type) (reflect.Value, bool) {
	if want == nil {
		return reflect.ValueOf(v), true
	}
	if v == nil {
		return reflect.Zero(want), true
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(want) {
		return rv, true
	}
	// A string parameter renders the value the way PHP renders it in a string
	// context. Go's own conversion is defined for every integer type and means
	// something else: reflect would turn int64(65) into "A" rather than "65".
	if want.Kind() == reflect.String {
		return reflect.ValueOf(phpString(v)).Convert(want), true
	}
	if rv.Type().ConvertibleTo(want) {
		return rv.Convert(want), true
	}
	return reflect.Value{}, false
}

// helperCall implements `base->method(args...)`. It dispatches PHP methods on
// model.Object instances back into the interpreter, and otherwise invokes a Go
// method on the value by reflection (the "invoke from values on the stack"
// capability the README relies on).
func (rt *Runtime) helperCall(ref *scopeRef) func(base any, method string, args ...any) (any, error) {
	return func(base any, method string, args ...any) (any, error) {
		scope := ref.scope
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
func (rt *Runtime) helperNew(ref *scopeRef) func(class string, args ...any) (any, error) {
	return func(class string, args ...any) (any, error) {
		scope := ref.scope
		if len(rt.observers) > 0 {
			defer rt.trace(scope, "new "+class)()
		}

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
		defaults := rt.classDefaultScope(c, scope)
		for _, f := range c.Fields {
			var def any
			if f.Default != nil {
				v, err := rt.Eval(f.Default, defaults)
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
		if value, ok := throwableMethod(base, method); ok {
			return value, nil
		}
		return nil, fmt.Errorf("undefined method %T::%s", base, method)
	}
	mt := m.Type()
	if wantsContext(mt) {
		args = append([]any{contextWithScope(contextWithEnv(rt.ctx, rt.Env), scope)}, args...)
	}
	in, err := buildArgs(mt, args, method)
	if err != nil {
		return nil, err
	}
	out := m.Call(in)
	return firstReturn(out)
}

// throwableMethod implements PHP's Throwable interface over any Go error.
//
// Every throwable class name is registered to one type, so a catch clause binds
// whatever error reached it: an Exception a script threw, an error a binding
// returned, or a panic converted at the host boundary. All of them answer the
// method set a script expects on a caught value. A method the concrete Go type
// defines wins, which is how *stdlib.Exception reports its own code.
func throwableMethod(base any, method string) (any, bool) {
	err, ok := base.(error)
	if !ok {
		return nil, false
	}
	switch strings.ToLower(strings.ReplaceAll(method, "_", "")) {
	case "getmessage", "tostring", "__tostring":
		return err.Error(), true
	case "getcode":
		return int64(0), true
	case "getprevious":
		if previous := errors.Unwrap(err); previous != nil {
			return previous, true
		}
		return nil, true
	case "getfile":
		return "", true
	case "getline":
		return int64(0), true
	case "gettrace":
		return model.NewArray(), true
	case "gettraceasstring":
		return "#0 {main}", true
	}
	return nil, false
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

// fieldByNameFold finds a struct field the way methodByNameFold finds a method:
// exact match first (PHP property access on a Go struct, `$rec->value` for the
// field Value), then case-insensitively, then without underscores so snake_case
// PHP property names resolve idiomatic Go names (is_readonly -> IsReadonly).
func fieldByNameFold(value reflect.Value, name string) reflect.Value {
	if f := value.FieldByName(name); f.IsValid() {
		return f
	}
	if f := value.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, name) }); f.IsValid() {
		return f
	}
	folded := strings.ReplaceAll(name, "_", "")
	return value.FieldByNameFunc(func(n string) bool {
		return strings.EqualFold(strings.ReplaceAll(n, "_", ""), folded)
	})
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
