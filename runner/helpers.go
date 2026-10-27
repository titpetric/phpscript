package runner

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/expr"
)

// This file implements the PHP-semantic helpers the expression engine calls:
// PHP's ordered hybrid arrays, lenient index/property access, dynamic method
// dispatch (PHP objects vs. forwarded Go values), and object construction.
// Keeping them in Go means PHP semantics have one home.

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
// A missing key answers nil, which is PHP's forgiving access, and raises nothing.
func helperIndex(base, idx any) any {
	switch b := base.(type) {
	case *model.Array:
		v, _ := b.Get(normalizeKey(idx))
		return v
	case model.Keyed:
		return b.Read(keyString(idx))
	case string, []byte:
		// String offset: $s[$i] is the byte at position i, as PHP's is and it
		// agrees with strlen and with the offsets strpos and
		// PREG_OFFSET_CAPTURE report. A negative offset counts from the end.
		// mb_substr($s, $i, 1) is the character read.
		//
		// A []byte takes the same branch, being the string it carries rather
		// than a list of integers. See phpval.Bytes.
		text := phpString(base)
		i := toInt(idx)
		if i < 0 {
			i += int64(len(text))
		}
		if i < 0 || i >= int64(len(text)) {
			return ""
		}
		return text[i : i+1]
	case nil:
		return nil
	}
	rv := reflect.ValueOf(base)
	switch rv.Kind() {
	case reflect.Map:
		// coerceArg converts the key to the map's own type first. A script
		// index is an int64 or a string and reading a map[int]T with the value
		// passed straight to MapIndex panics and answers nothing.
		mapKey, ok := coerceArg(rv.Type().Key(), normalizeKey(idx))
		if !ok || !mapKey.Type().AssignableTo(rv.Type().Key()) {
			return nil
		}
		mv := rv.MapIndex(mapKey)
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
// a *Callable bound to that object; this supports PHP idioms such as
// `call_user_func_array($this->query, $args)`, and a router takes one as a
// handler registered as `$this->fnName`.
func (rt *Runtime) helperGet(ref *scopeRef) func(base any, name string) any {
	return func(base any, name string) any {
		// A bound callable produced here can outlive the expression that read it,
		// and the closure copies the scope by value and not through the reference.
		scope := ref.scope
		switch b := base.(type) {
		case *model.Object:
			if v, ok := b.Props[name]; ok {
				return v
			}
			if b.Class != nil {
				if decl, ok := b.Class.Methods[name]; ok {
					return rt.newMethod(b, decl, scope)
				}
			}
			return nil
		case nil:
			return nil
		}
		rv := reflect.ValueOf(base)
		value := reflect.Indirect(rv)
		if value.Kind() == reflect.Struct {
			if index, ok := rt.structField(value.Type(), name); ok {
				if f, err := value.FieldByIndexErr(index); err == nil {
					return f.Interface()
				}
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
		return rt.callGoMethod(base, method, args, func() *Scope {
			if scope != nil {
				return scope
			}
			return rt.newScope()
		})
	}
}

// exprHelpers returns the typed helper implementations the engine installs.
// Package-level because the helpers are stateless: a compiled closure chain
// is shareable across runtimes. PanicError mirrors invokeAny's recover and
// a host panic surfaces as the same catchable error either way.
var exprHelpers = &expr.Helpers{
	Truthy:     phpTruthy,
	Concat:     helperConcat,
	Str:        phpString,
	Pair:       helperPair,
	Array:      helperArray,
	Index:      helperIndex,
	Cast:       helperCast,
	Arith:      phpArith,
	Compare:    phpCompare,
	Bitwise:    phpBitwise,
	BitNot:     phpBitNot,
	InstanceOf: phpInstanceOf,
	Negate:     phpNegate,
	PanicError: func(recovered any) error {
		return &HostPanicError{Callable: "expr", Value: recovered}
	},
	Slotted: isSlotIdent,
}

// isSlotIdent reports whether an identifier is one Eval binds per evaluation:
// a PHP variable (varIdent: `v_` or bare `this`), a bare-name constant
// (constIdent: `c_`), or a closure literal (`__cl<N>`). The digit check
// keeps `__classconst`, which shares the `__cl` prefix, on the Base path with
// the other helpers.
func isSlotIdent(name string) bool {
	if name == "this" {
		return true
	}
	if strings.HasPrefix(name, "v_") || strings.HasPrefix(name, constIdentPrefix) {
		return true
	}
	if len(name) > 4 && name[:4] == "__cl" {
		for i := 4; i < len(name); i++ {
			if name[i] < '0' || name[i] > '9' {
				return false
			}
		}
		return true
	}
	return false
}

// helperCall implements `base->method(args...)`. It dispatches PHP methods on
// model.Object instances back into the interpreter, and otherwise invokes a Go
// method on the value by reflection (the "invoke from values on the stack"
// capability the README relies on).
func (rt *Runtime) helperCall(ref *scopeRef) func(base any, methodValue any, args ...any) (any, error) {
	return func(base any, methodValue any, args ...any) (any, error) {
		scope := ref.scope
		// `$obj->$m(...)` arrives with the method name as a runtime value.
		method := phpString(methodValue)
		if obj, ok := base.(*model.Object); ok && obj.Class != nil {
			if decl, ok := lookupPHPMethod(obj.Class, method); ok {
				// The frame keeps its arguments for func_get_args, and the
				// slice arriving here is the expression arena's, borrowed for
				// the call: the frame gets its own. expr.scratchName names
				// this clone as what makes __call safe on the arena.
				return rt.invokeMethod(obj, decl, slices.Clone(args), scope)
			}
		}
		return rt.callGoMethod(base, method, args, func() *Scope { return scope })
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
func (rt *Runtime) helperNew(ref *scopeRef) func(classValue any, args ...any) (any, error) {
	return func(classValue any, args ...any) (any, error) {
		scope := ref.scope
		// `new $className(...)` arrives with the class as a runtime value: a
		// string name, or an instance whose class it reuses, as PHP allows.
		var class string
		switch v := classValue.(type) {
		case string:
			class = v
		case *model.Object:
			if v.Class != nil {
				class = v.Class.Name
			}
		}
		if class == "" {
			return nil, fmt.Errorf("new: class name must be a string, got %T", classValue)
		}
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
			v, err := rt.invokeEntry(ctor, args, scope)
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
			obj.SetProp(f.Name, def)
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
