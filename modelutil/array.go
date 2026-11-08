package modelutil

import (
	"github.com/titpetric/phpscript/model"
)

// ToArray returns v as an *model.Array, converting a native collection if
// needed and passing an existing one through untouched. Use it at the point
// where PHP semantics genuinely require an array (mutation, `$a[] = v`), not
// to normalise arguments, since RangeValues reads every shape without
// allocating.
func ToArray(v any) *model.Array {
	if arr, ok := v.(*model.Array); ok && arr != nil {
		return arr
	}
	if items, ok := v.([]any); ok {
		out := model.NewArraySize(len(items))
		out.AppendAll(items)
		return out
	}
	n, _ := LenValues(v)
	out := model.NewArraySize(n)
	RangeValues(v, func(key, val any) bool {
		if i, ok := key.(int64); ok && i == out.NextIndex() {
			out.Append(val)
		} else {
			out.Set(key, val)
		}
		return true
	})
	return out
}
