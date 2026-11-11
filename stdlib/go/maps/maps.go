// Package maps binds the day-to-day functions of go's maps package as the
// Runtime\Go\Maps namespace. A script imports it with `use Runtime\Go\Maps;`
// and calls `Maps\Keys($m)`; the default runtime defines none of these names.
// A php array keeps its keys in order. Every function here reads and answers in
// that order, which go's maps do not.
package maps

import (
	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

// Namespace is the name a script imports.
const Namespace = modelutil.GoNamespace + `\Maps`

func init() {
	runner.RegisterNamespace(Namespace, Register)
}

func name(fn string) string { return Namespace + `\` + fn }

// clone copies the entries of a collection into a new array, keys in order.
// The result is an array to keep the keys.
func clone(m any) *model.Array {
	out := model.NewArray()
	modelutil.RangeValues(m, func(k, v any) bool {
		out.Set(phpval.Key(k), v)
		return true
	})
	return out
}

// Register installs the Runtime\Go\Maps functions on a runtime that imported them.
func Register(rt *runner.Runtime) {
	// Runtime\Go\Maps\Keys returns the keys of $m in order.
	rt.RegisterFunc(name("Keys"), func(m any) []any {
		out := []any{}
		modelutil.RangeValues(m, func(k, _ any) bool { out = append(out, k); return true })
		return out
	})
	// Runtime\Go\Maps\Values returns the values of $m in order.
	rt.RegisterFunc(name("Values"), func(m any) []any { return append([]any{}, phpval.Values(m)...) })
	// Runtime\Go\Maps\Clone returns a copy of $m.
	rt.RegisterFunc(name("Clone"), clone)
	// Runtime\Go\Maps\Equal reports whether two maps hold the same keys with equal values.
	rt.RegisterFunc(name("Equal"), func(a, b any) bool {
		x, y := clone(a), clone(b)
		if x.Len() != y.Len() {
			return false
		}
		equal := true
		x.Range(func(k, v any) bool {
			w, ok := y.Get(k)
			equal = ok && phpval.Compare(v, w) == 0
			return equal
		})
		return equal
	})
	// Runtime\Go\Maps\Copy returns $dst with every entry of $src set over it.
	rt.RegisterFunc(name("Copy"), func(dst, src any) *model.Array {
		out := clone(dst)
		modelutil.RangeValues(src, func(k, v any) bool {
			out.Set(phpval.Key(k), v)
			return true
		})
		return out
	})
}
