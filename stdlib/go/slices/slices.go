// Package slices binds the day-to-day functions of go's slices package as the
// Runtime\Go\Slices namespace. A script imports it with `use Runtime\Go\Slices;`
// and calls `Slices\Contains($list, $v)`; the default runtime carries none of
// these names. A function reads a list and answers a new one, because a php
// array is a value and nothing is sorted or reversed in place.
package slices

import (
	"errors"
	"reflect"
	goslices "slices"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// Namespace is the name a script imports.
const Namespace = model.GoNamespace + `\Slices`

func init() {
	runner.RegisterNamespace(Namespace, Register)
}

func name(fn string) string { return Namespace + `\` + fn }

// same is go's ==: equal in value and of one type, so 1 and "1" differ.
func same(a, b any) bool {
	return phpval.Compare(a, b) == 0 && reflect.TypeOf(a) == reflect.TypeOf(b)
}

// Register installs the Runtime\Go\Slices functions on a runtime that imported them.
func Register(rt *runner.Runtime) {
	// Runtime\Go\Slices\Contains reports whether $v is present in $s.
	rt.RegisterFunc(name("Contains"), func(s, v any) bool {
		return goslices.ContainsFunc(phpval.Values(s), func(e any) bool { return same(e, v) })
	})
	// Runtime\Go\Slices\Index returns the index of the first $v in $s, or -1.
	rt.RegisterFunc(name("Index"), func(s, v any) int {
		return goslices.IndexFunc(phpval.Values(s), func(e any) bool { return same(e, v) })
	})
	// Runtime\Go\Slices\Equal reports whether two lists hold the same values in order.
	rt.RegisterFunc(name("Equal"), func(a, b any) bool { return goslices.EqualFunc(phpval.Values(a), phpval.Values(b), same) })
	// Runtime\Go\Slices\Reverse returns the list in reverse order.
	rt.RegisterFunc(name("Reverse"), func(s any) []any {
		out := goslices.Clone(phpval.Values(s))
		goslices.Reverse(out)
		return out
	})
	// Runtime\Go\Slices\Sort returns the list in ascending order, as php compares values.
	rt.RegisterFunc(name("Sort"), func(s any) []any {
		out := goslices.Clone(phpval.Values(s))
		goslices.SortStableFunc(out, phpval.Compare)
		return out
	})
	// Runtime\Go\Slices\Max returns the largest value of a list that is not empty.
	rt.RegisterFunc(name("Max"), func(s any) (any, error) {
		vals := phpval.Values(s)
		if len(vals) == 0 {
			return nil, errors.New("slices.Max: empty list")
		}
		return vals[maxIndex(vals)], nil
	})
	// Runtime\Go\Slices\Min returns the smallest value of a list that is not empty.
	rt.RegisterFunc(name("Min"), func(s any) (any, error) {
		vals := phpval.Values(s)
		if len(vals) == 0 {
			return nil, errors.New("slices.Min: empty list")
		}
		return vals[minIndex(vals)], nil
	})
	// Runtime\Go\Slices\Compact drops the repeats that sit next to each other.
	rt.RegisterFunc(name("Compact"), func(s any) []any { return goslices.CompactFunc(goslices.Clone(phpval.Values(s)), same) })
	// Runtime\Go\Slices\Concat joins the lists into one.
	rt.RegisterFunc(name("Concat"), func(lists ...any) []any {
		out := []any{}
		for _, l := range lists {
			out = append(out, phpval.Values(l)...)
		}
		return out
	})
}

func maxIndex(vals []any) int {
	best := 0
	for i, v := range vals {
		if phpval.Compare(v, vals[best]) > 0 {
			best = i
		}
	}
	return best
}

func minIndex(vals []any) int {
	best := 0
	for i, v := range vals {
		if phpval.Compare(v, vals[best]) < 0 {
			best = i
		}
	}
	return best
}
