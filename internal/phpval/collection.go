package phpval

import (
	"github.com/titpetric/phpscript/model"
)

// Strings returns a collection's values as strings in order.
func Strings(a any) []string {
	if parts, ok := a.([]string); ok {
		return parts
	}
	n, _ := model.LenValues(a)
	if n == 0 {
		return nil
	}
	out := make([]string, 0, n)
	model.RangeValues(a, func(_, v any) bool { out = append(out, String(v)); return true })
	return out
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
