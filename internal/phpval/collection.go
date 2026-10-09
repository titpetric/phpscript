package phpval

import (
	"github.com/titpetric/phpscript/model"
)

// Strings returns a collection's values as strings in order.
//
// Every caller is a string context - implode and str_replace - so an element
// with no string form is the error StringContext reports, and no spelling
// invented for it.
func Strings(a any) ([]string, error) {
	if parts, ok := a.([]string); ok {
		return parts, nil
	}
	n, _ := model.LenValues(a)
	if n == 0 {
		return nil, nil
	}
	// One captured struct and no two captured variables: each variable a
	// closure captures by reference is its own heap allocation, and this runs
	// under implode. The refused value is carried as its concrete type so the
	// happy path stores nothing.
	state := struct {
		out     []string
		refused *model.Object
	}{out: make([]string, 0, n)}
	model.RangeValues(a, func(_, v any) bool {
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

// Values returns a collection's values in order. A []any is returned as is:
// callers only read it, and the shims that do not (array_splice) copy first.
func Values(a any) []any {
	if vals, ok := a.([]any); ok {
		return vals
	}
	n, _ := model.LenValues(a)
	if n == 0 {
		return nil
	}
	out := make([]any, 0, n)
	model.RangeValues(a, func(_, v any) bool { out = append(out, v); return true })
	return out
}
