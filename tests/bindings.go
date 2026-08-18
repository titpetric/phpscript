package tests

// This file is the reference set of *binding return shapes*. Every function
// here answers the same question, "how should a forwarded Go function hand a
// collection back to the PHP VM?", with a different Go type, so that
// bindings_test.go can assert both semantics (does the VM understand it?) and
// cost (how many allocations did it take?).
//
// The runtime never requires *model.Array on the way out. Registered functions
// are invoked through reflection (runner.invokeAny), and the single return
// value is boxed into `any` by firstReturn. Everything downstream, from
// foreach to `$x[...]`, `$obj->field` and method calls, dispatches on the dynamic
// type, with reflection fallbacks for native Go slices, maps and structs.
// So a binding is free to return the cheapest representation of its data and
// let the VM reflect over it.
//
// The shapes, cheapest first:
//
//	[]string / []T      one allocation, no per-element boxing
//	[]any               one allocation, boxes each element
//	map[string]any      one allocation (+ buckets), boxes each value
//	*model.Array        struct + map[any]any + keys slice, boxes keys AND values
//
// *model.Array is the only shape with PHP array semantics (ordered, hybrid
// int/string keys, mutable in place), so it stays the right answer for values
// PHP will mutate or key by string in insertion order. It is the wrong answer
// for a function that just returns a list the script will iterate.

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// toString renders a value the way the PHP VM would in a string context, for
// the comparator binding below.
func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// BindingRecord is a plain Go struct handed to PHP as an object. Property
// access goes through runner.helperGet, which reads exported fields by
// reflection (case-insensitively), so `$r->name` and `$r->Name` both resolve.
type BindingRecord struct {
	ID   int64
	Name string
}

// Label is an exported method so tests can exercise `$r->label()` dispatch on a
// forwarded Go value.
func (r BindingRecord) Label() string {
	return r.Name
}

// bindingWords is the fixed input used by every list-returning binding, so the
// benchmarks compare representations rather than workloads.
var bindingWords = []string{"alpha", "beta", "gamma", "delta", "epsilon"}

// RegisterBindings installs the example bindings. It is passed to
// stdlib.Register as an extra installer, the same way a host contributes its
// own symbols.
func RegisterBindings(rt *runner.Runtime) {
	for name, fn := range BindingFuncs() {
		rt.RegisterFunc(name, fn)
	}
}

// BindingFunc returns the raw (unregistered, unadapted) Go function behind a
// binding name, so benchmarks can measure the reflection return path on its own.
func BindingFunc(name string) any {
	return BindingFuncs()[name]
}

// BindingFuncs returns the example bindings keyed by the PHP name they are
// registered under.
func BindingFuncs() map[string]any {
	funcs := map[string]any{}
	rt := &collector{funcs: funcs}
	registerBindings(rt)
	return funcs
}

// collector captures RegisterFunc calls without a Runtime, so the same binding
// definitions serve both the VM tests and the raw-call benchmarks.
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
	// Same data, four representations. bind_list_array is what stdlib used to
	// do everywhere; bind_list_strings is the cheap end of the scale.

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
	// bind_list_shared returns the backing slice with no copy at all: zero
	// allocations for a read-only result. PHP arrays are value types, so this
	// is only correct when the script does not mutate the result, which is
	// also true of every *model.Array a binding returns today (those are
	// handed out by pointer).
	rt.RegisterFunc("bind_list_shared", func() []string { return bindingWords })

	// --- the `any` question ------------------------------------------------
	//
	// Declaring the return type as `any` rather than a concrete type costs
	// nothing: firstReturn calls reflect.Value.Interface() either way, and a
	// slice header already lives behind an interface once returned. These two
	// exist so the benchmark can prove that rather than assert it.

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

	// --- struct shapes -----------------------------------------------------
	//
	// A struct pointer is the cheapest possible object: one allocation, no
	// property map. Returning a value (not a pointer) costs one boxing
	// allocation, and the copy makes field writes silently useless, so prefer
	// pointers for anything PHP might assign to.

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
	// For comparison: what a PHP-side object costs (class pointer + props map).

	bindingClass := &model.Class{Name: "BindingRecord", Methods: map[string]*model.FuncDecl{}}
	rt.RegisterFunc("bind_object", func() *model.Object {
		obj := model.NewObject(bindingClass)
		obj.Props["id"] = int64(1)
		obj.Props["name"] = "alpha"
		return obj
	})

	// --- scalar shapes -----------------------------------------------------
	//
	// Scalars are where `any` actually costs something: returning a concrete
	// int64 lets firstReturn box a value the compiler may have already staged,
	// while `any` boxes at the return statement. Both allocate 8 bytes unless
	// the value is a small int (0-255) served from the runtime's static cache.

	rt.RegisterFunc("bind_int", func() int64 { return 4096 })
	rt.RegisterFunc("bind_int_any", func() any { return int64(4096) })
	rt.RegisterFunc("bind_small_int", func() int64 { return 7 })
	rt.RegisterFunc("bind_string", func() string { return "alpha" })
	rt.RegisterFunc("bind_bool", func() bool { return true })

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

	// --- stdlib before/after ----------------------------------------------
	//
	// bind_explode_legacy is the *model.Array implementation stdlib's explode()
	// used to have, kept so the benchmarks measure the change rather than
	// asserting it. bind_explode_native is what it does now.

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
	// A trailing error is unwrapped by firstReturn and surfaces as a thrown
	// PHP error. Returning (any, error) is only worth it when the call can
	// actually fail; a second return value costs an extra reflect.Value slot.

	rt.RegisterFunc("bind_split", func(s, sep string) ([]string, error) {
		if sep == "" {
			return nil, errEmptySeparator
		}
		return strings.Split(s, sep), nil
	})
}

// errEmptySeparator is returned by bind_split to exercise the error path.
var errEmptySeparator = bindingError("bind_split: empty separator")

type bindingError string

func (e bindingError) Error() string { return string(e) }
