package tests

// This file is the reference set of *binding return shapes*. Every function
// here answers one question: how does a forwarded Go function return a
// collection to the PHP VM? Each uses a different Go type. bindings_test.go can
// assert semantics (does the VM understand the value?) and cost (how many
// allocations did it take?).
//
// The runtime never requires *model.Array on the way out. runner.invokeAny
// calls a registered function through reflection, and callResult boxes the
// single return value into `any`. Everything downstream, from foreach to
// `$x[...]`, `$obj->field` and method calls, dispatches on the dynamic type,
// with reflection fallbacks for native Go slices, maps and structs. So a
// binding is free to return the cheapest representation of its data and let
// the VM reflect over it.
//
// The shapes, cheapest first:
//
//	[]string / []T      one allocation, no per-element boxing
//	[]any               one allocation, boxes each element
//	map[string]any      one allocation (+ buckets), boxes each value
//	*model.Array        struct + map[any]any + keys slice, boxes keys AND values
//
// *model.Array is the only shape with PHP array semantics (ordered, hybrid
// int/string keys, mutable in place) and it stays the right answer for values
// PHP will mutate or key by string in insertion order. A function returning a
// list the script iterates has no use for it.

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// toString renders a value as the PHP VM would in a string context.
func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// BindingRecord is a plain Go struct handed to PHP as an object. Property
// access resolves through runner.helperGet, which reads exported fields by
// reflection (case-insensitively) and `$r->name` and `$r->Name` both resolve.
type BindingRecord struct {
	ID   int64
	Name string
}

// Label exercises `$r->label()` dispatch on a forwarded Go value.
func (r BindingRecord) Label() string {
	return r.Name
}

// bindingWords is the fixed input every list-returning binding shares.
var bindingWords = []string{"alpha", "beta", "gamma", "delta", "epsilon"}

// RegisterBindings installs the example bindings. stdlib.Register takes it as
// an extra installer, the same way a host adds its symbols.
func RegisterBindings(rt *runner.Runtime) {
	for name, fn := range BindingFuncs() {
		rt.RegisterFunc(name, fn)
	}
}

// BindingFunc returns the unadapted Go function behind a binding name.
func BindingFunc(name string) any {
	return BindingFuncs()[name]
}

// BindingFuncs returns the example bindings keyed by the PHP name each
// registers under.
func BindingFuncs() map[string]any {
	funcs := map[string]any{}
	rt := &collector{funcs: funcs}
	registerBindings(rt)
	return funcs
}

// collector captures RegisterFunc calls so bindings serve the VM tests and raw benchmarks.
type collector struct {
	funcs map[string]any
}

func (c *collector) RegisterFunc(name string, fn any) { c.funcs[name] = fn }

// registrar is the slice of *runner.Runtime that binding definitions need.
type registrar interface {
	RegisterFunc(name string, fn any)
}

func registerBindings(rt registrar) {
	// --- list shapes -------------------------------------------------------
	//
	// Same data, four representations. bind_list_array is the expensive end of
	// the scale; bind_list_strings is the cheap end.

	rt.RegisterFunc("bind_list_array", func() *model.Array {
		out := model.NewArray()
		for _, w := range bindingWords {
			out.Append(w)
		}
		return out
	})
	rt.RegisterFunc("bind_list_strings", func() []string {
		out := make([]string, len(bindingWords))
		copy(out, bindingWords)
		return out
	})
	rt.RegisterFunc("bind_list_any", func() []any {
		out := make([]any, 0, len(bindingWords))
		for _, w := range bindingWords {
			out = append(out, w)
		}
		return out
	})
	// bind_list_shared returns the backing slice without copying: zero
	// allocations for a read-only result. PHP arrays are value types. This is
	// only correct when the script does not mutate the result. That also
	// holds for every *model.Array a binding returns today, all by pointer.
	rt.RegisterFunc("bind_list_shared", func() []string { return bindingWords })

	// --- the `any` question ------------------------------------------------
	//
	// Declaring the return type as `any` in place of a concrete type costs no
	// allocation: callResult calls reflect.Value.Interface() either way. A
	// slice header already lives behind an interface once returned. These two
	// exist so the benchmark can prove that, and assert nothing.

	rt.RegisterFunc("bind_any_strings", func() any {
		out := make([]string, len(bindingWords))
		copy(out, bindingWords)
		return out
	})
	rt.RegisterFunc("bind_any_array", func() any {
		out := model.NewArray()
		for _, w := range bindingWords {
			out.Append(w)
		}
		return out
	})

	// --- map / row shapes --------------------------------------------------
	//
	// The database bindings' hot path: N rows of M columns. Returning
	// []map[string]any skips two allocations and 2*M interface boxes per row
	// compared with the nested *model.Array conversion.

	rt.RegisterFunc("bind_rows_array", func() *model.Array {
		rows := model.NewArray()
		for i, w := range bindingWords {
			row := model.NewArray()
			row.Set("id", int64(i))
			row.Set("name", w)
			rows.Append(row)
		}
		return rows
	})
	rt.RegisterFunc("bind_rows_maps", func() []map[string]any {
		rows := make([]map[string]any, 0, len(bindingWords))
		for i, w := range bindingWords {
			rows = append(rows, map[string]any{"id": int64(i), "name": w})
		}
		return rows
	})
	rt.RegisterFunc("bind_map", func() map[string]any {
		return map[string]any{"id": int64(1), "name": "alpha"}
	})
	// A list a binding keyed by position, never packed into a slice. The
	// shape keeps PHP's numbering through an unset; a map holds the hole a
	// slice cannot.
	rt.RegisterFunc("bind_list_keyed", func() map[int]string {
		keyed := make(map[int]string, len(bindingWords))
		for i, w := range bindingWords {
			keyed[i] = w
		}
		return keyed
	})

	// --- struct shapes -----------------------------------------------------
	//
	// A struct pointer is the cheapest possible object: one allocation, no
	// property map. Returning a value costs one boxing allocation. The copy
	// makes field writes silently useless. Return a pointer for anything PHP
	// might assign to.

	rt.RegisterFunc("bind_record", func() *BindingRecord {
		return &BindingRecord{ID: 1, Name: "alpha"}
	})
	rt.RegisterFunc("bind_record_value", func() BindingRecord {
		return BindingRecord{ID: 1, Name: "alpha"}
	})
	rt.RegisterFunc("bind_records", func() []BindingRecord {
		out := make([]BindingRecord, 0, len(bindingWords))
		for i, w := range bindingWords {
			out = append(out, BindingRecord{ID: int64(i), Name: w})
		}
		return out
	})

	// --- object shape ------------------------------------------------------
	//
	// For comparison: the cost of a PHP-side object (class pointer + props
	// map).

	bindingClass := &model.Class{Name: "BindingRecord", Methods: map[string]*model.FuncDecl{}}
	rt.RegisterFunc("bind_object", func() *model.Object {
		obj := model.NewObject(bindingClass)
		obj.SetProp("id", int64(1))
		obj.SetProp("name", "alpha")
		return obj
	})

	// --- scalar shapes -----------------------------------------------------
	//
	// Scalars are where `any` actually costs something: returning a concrete
	// int64 lets callResult box a value the compiler may have already staged,
	// while `any` boxes at the return statement. Both cost an 8 byte allocation
	// unless the value is a small int (0-255) from the runtime's static cache.

	rt.RegisterFunc("bind_int", func() int64 { return 4096 })
	rt.RegisterFunc("bind_int_any", func() any { return int64(4096) })
	rt.RegisterFunc("bind_small_int", func() int64 { return 7 })
	rt.RegisterFunc("bind_string", func() string { return "alpha" })
	rt.RegisterFunc("bind_bool", func() bool { return true })
	// A []byte is a string. Treating it as a list of integers is wrong: PHP's
	// strings are byte strings, and Go's own text APIs declare []byte half the
	// time. regexp.Regexp.Find is the case that brought it in.
	rt.RegisterFunc("bind_bytes", func() []byte { return []byte("alpha") })
	rt.RegisterFunc("bind_bytes_empty", func() []byte { return nil })
	rt.RegisterFunc("bind_bytes_list", func() [][]byte {
		out := make([][]byte, 0, len(bindingWords))
		for _, w := range bindingWords {
			out = append(out, []byte(w))
		}
		return out
	})
	// bind_compare_desc is a comparator for usort, to check that sorting a
	// binding's slice mutates the slice the script holds.
	rt.RegisterFunc("bind_compare_desc", func(a, b any) int64 {
		x, y := toString(a), toString(b)
		switch {
		case x > y:
			return -1
		case x < y:
			return 1
		default:
			return 0
		}
	})

	// --- callback shapes ---------------------------------------------------
	//
	// One binding for each callback shape. The uniform shape matches
	// Runtime.Callable's; the rest use Go's terms, already matched by a
	// library's method set. reflect.MakeFunc fills them over the declared
	// signature.

	// bind_callback_uniform declares the shape Runtime.Callable answers. A
	// binding gets it without a reflect.MakeFunc wrapper. It alone has a slot
	// for an error the callback reports.
	rt.RegisterFunc("bind_callback_uniform", func(s string, fn func(...any) (any, error)) (any, error) {
		return fn(s)
	})
	// bind_callback is the mapper shape, as regexp.Regexp's
	// ReplaceAllStringFunc declares it. A binding declares this shape most
	// often; coerceArgOn could not fill it at all.
	rt.RegisterFunc("bind_callback", func(s string, fn func(string) string) string {
		return fn(s)
	})
	// bind_callback_bool is the predicate shape, where a PHP value does not
	// always fit the declared result.
	rt.RegisterFunc("bind_callback_bool", func(s string, fn func(string) bool) string {
		if fn(s) {
			return "kept"
		}
		return "dropped"
	})
	// bind_callback_void declares no result at all and nothing reads or
	// coerces the value the callable returns.
	rt.RegisterFunc("bind_callback_void", func(s string, fn func(string)) string {
		fn(s)
		return "done"
	})
	// bind_callback_two_results declares a shape a PHP callable cannot fill: a
	// closure answers one value and the second result has nothing to come from.
	rt.RegisterFunc("bind_callback_two_results", func(fn func(string) (string, bool)) string {
		s, _ := fn("alpha")
		return s
	})

	// --- stdlib before/after ----------------------------------------------
	//
	// bind_explode_legacy is the *model.Array implementation stdlib's explode()
	// had before. The benchmarks measure the change without asserting it.
	// bind_explode_native is the current implementation.

	rt.RegisterFunc("bind_explode_legacy", func(delim, s string) *model.Array {
		out := model.NewArray()
		for _, part := range strings.Split(s, delim) {
			out.Append(part)
		}
		return out
	})
	rt.RegisterFunc("bind_explode_native", func(delim, s string) []string {
		return strings.Split(s, delim)
	})

	// --- error shape -------------------------------------------------------
	//
	// callResult unwraps a trailing error and surfaces it as a thrown PHP
	// error. Returning (any, error) earns its keep only when the call can
	// actually fail; a second return value costs an extra reflect.Value slot.

	rt.RegisterFunc("bind_split", func(s, sep string) ([]string, error) {
		if sep == "" {
			return nil, errEmptySeparator
		}
		return strings.Split(s, sep), nil
	})
}

// errEmptySeparator is the error bind_split returns to exercise the error path.
var errEmptySeparator = bindingError("bind_split: empty separator")

type bindingError string

func (e bindingError) Error() string { return string(e) }
