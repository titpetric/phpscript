package phpval

import (
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// Strings returns a collection's values as strings in order.
//
// Every caller is a string context - implode and str_replace. An element
// lacking a string form returns the error StringContext reports.
func Strings(a any) ([]string, error) {
	if parts, ok := a.([]string); ok {
		return parts, nil
	}
	n, _ := modelutil.LenValues(a)
	if n == 0 {
		return nil, nil
	}
	// One captured struct, never two captured variables: each variable a
	// closure captures by reference costs a separate heap allocation. This runs
	// under implode. The field holds the refused value as its concrete type,
	// and the happy path stores nothing.
	state := struct {
		out     []string
		refused *model.Object
	}{out: make([]string, 0, n)}
	modelutil.RangeValues(a, func(_, v any) bool {
		if object, ok := v.(*model.Object); ok {
			state.refused = object
			return false
		}
		state.out = append(state.out, String(v))
		return true
	})
	if state.refused != nil {
		return nil, &ConversionError{Class: classNameOf(state.refused)}
	}
	return state.out, nil
}

// Values returns a collection's values in order. A []any comes back as it
// stands: callers only read it. The shims that do not (array_splice) copy
// first.
func Values(a any) []any {
	if vals, ok := a.([]any); ok {
		return vals
	}
	n, _ := modelutil.LenValues(a)
	if n == 0 {
		return nil
	}
	out := make([]any, 0, n)
	modelutil.RangeValues(a, func(_, v any) bool { out = append(out, v); return true })
	return out
}
