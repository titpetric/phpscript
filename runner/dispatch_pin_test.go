package runner

// This file pins the observable behaviour of the call-dispatch machinery
// (invokeAny with its invokeFast shortcut, adapt, buildArgs/coerceArg,
// callGoMethod, wantsContext, and the evaluation-environment helpers) ahead
// of its consolidation into registration-time invokers. Every test here holds
// before and after that refactor: the assertions go through invokeAny, adapt,
// callGoMethod and the public API, never through the shortcut directly.
//
// The one lever the tests use is Go's type identity: invokeFast's type switch
// matches concrete unnamed signatures only, so a defined func type with the
// same underlying signature is invisible to it and always takes the reflect
// path. Calling the same behaviour through both spellings is what turns
// "fast equals reflect" into an assertion.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

// Named counterparts of every signature the invokeFast type switch covers.
// A value of one of these types carries the same underlying signature but a
// different dynamic type, which is what sends it down the reflect path.
type (
	dispatchPinVarAnyRetAnyErr       func(...any) (any, error)
	dispatchPinAnyRetAny             func(any) any
	dispatchPinAnyRetBool            func(any) bool
	dispatchPinAnyRetString          func(any) string
	dispatchPinAnyAnyRetString       func(any, any) string
	dispatchPinAnyAnyRetAny          func(any, any) any
	dispatchPinStrRetStr             func(string) string
	dispatchPinRetStr                func() string
	dispatchPinRetAny                func() any
	dispatchPinVarAnyRetAny          func(...any) any
	dispatchPinVarAnyRetBool         func(...any) bool
	dispatchPinStrRetAny             func(string) any
	dispatchPinStrRetBool            func(string) bool
	dispatchPinStrRetInt             func(string) int64
	dispatchPinRetInt                func() int64
	dispatchPinStrStrRetBool         func(string, string) bool
	dispatchPinStrStrRetStr          func(string, string) string
	dispatchPinStrStrRetBoolErr      func(string, string) (bool, error)
	dispatchPinAnyRetInt             func(any) int64
	dispatchPinAnyRetFloat           func(any) float64
	dispatchPinAnyRetAnyErr          func(any) (any, error)
	dispatchPinAnyRetBoolErr         func(any) (bool, error)
	dispatchPinAnyAnyRetBoolErr      func(any, any) (bool, error)
	dispatchPinAnyVarAnyRetAnyErr    func(any, ...any) (any, error)
	dispatchPinAnyVarAnyRetArr       func(any, ...any) *model.Array
	dispatchPinStrVarAnyRetStr       func(string, ...any) string
	dispatchPinStrVarStrRetStr       func(string, ...string) string
	dispatchPinRetArr                func() *model.Array
	dispatchPinAnyAnyVarAnyRetAnyErr func(any, any, ...any) (any, error)
	dispatchPinAnyStrRetAny          func(any, string) any
	dispatchPinStrAnyRetAnyErr       func(string, any) (any, error)
	dispatchPinStrAnyVarAnyRetAnyErr func(string, any, ...any) (any, error)
	dispatchPinStrStrVarAnyRetAnyErr func(string, string, ...any) (any, error)
	dispatchPinStrStrRetAnyErr       func(string, string) (any, error)
	dispatchPinStrBoolRetAnyErr      func(string, bool) (any, error)
	dispatchPinStrRetFunc            func(string) func(any)
)

// dispatchPinErr is the error the error-returning probe bindings hand back
// when asked to, so both dispatch paths can be checked for surfacing it.
var dispatchPinErr = errors.New("dispatch pin: forced failure")

// dispatchPinSink is the func value the func(string) func(any) probe returns.
// Package-level so both dispatch paths return the identical value and the
// comparison can be by pointer rather than by an unobservable closure.
var dispatchPinSink = func(any) {}

// dispatchPinArgs spells out each argument as value and dynamic type, so a
// probe's return value records exactly what the binding received: "65(string)"
// proves the int64 was coerced, "<nil>(<nil>)" proves the padding arrived.
func dispatchPinArgs(vs ...any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprintf("%v(%T)", v, v)
	}
	return strings.Join(parts, ",")
}

// dispatchPinBoom reports whether a probe was asked to fail.
func dispatchPinBoom(v any) bool { return fmt.Sprint(v) == "boom" }

// dispatchPinArray builds the array the *model.Array probes return, one
// stringified element per argument, so two runs of the same probe build
// structurally equal arrays.
func dispatchPinArray(vs ...any) *model.Array {
	arr := model.NewArraySize(len(vs))
	for _, v := range vs {
		arr.Append(fmt.Sprintf("%v(%T)", v, v))
	}
	return arr
}

// dispatchPinEqual compares two dispatch results. Func results compare by type
// and code pointer (DeepEqual calls all non-nil funcs unequal); everything
// else, arrays included, compares structurally.
func dispatchPinEqual(a, b any) bool {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if av.IsValid() && bv.IsValid() && av.Kind() == reflect.Func && bv.Kind() == reflect.Func {
		return av.Type() == bv.Type() && av.Pointer() == bv.Pointer()
	}
	return reflect.DeepEqual(a, b)
}

// Every signature the fast path recognises produces the same value and the
// same error as the reflect path, for full calls, short calls (padding), nil
// arguments, an int64 where a string parameter sits, and surplus arguments.
// This is the contract a consolidated dispatcher has to keep: which path a
// call takes must never be observable from a script.
func TestDispatchPinFastReflectParity(t *testing.T) {
	tests := []struct {
		name    string
		fast    any
		slow    any
		argSets [][]any
	}{
		{
			name: "func(...any) (any, error)",
			fast: func(vs ...any) (any, error) {
				if len(vs) > 0 && dispatchPinBoom(vs[0]) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(vs...), nil
			},
			slow: dispatchPinVarAnyRetAnyErr(func(vs ...any) (any, error) {
				if len(vs) > 0 && dispatchPinBoom(vs[0]) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(vs...), nil
			}),
			argSets: [][]any{{}, {int64(65), nil, "x"}, {"boom"}},
		},
		{
			name:    "func(any) any",
			fast:    func(v any) any { return "one:" + dispatchPinArgs(v) },
			slow:    dispatchPinAnyRetAny(func(v any) any { return "one:" + dispatchPinArgs(v) }),
			argSets: [][]any{{}, {nil}, {int64(65)}, {"a", "b"}},
		},
		{
			name:    "func(any) bool",
			fast:    func(v any) bool { return v != nil },
			slow:    dispatchPinAnyRetBool(func(v any) bool { return v != nil }),
			argSets: [][]any{{}, {int64(0)}, {"x"}},
		},
		{
			name:    "func(any) string",
			fast:    func(v any) string { return dispatchPinArgs(v) },
			slow:    dispatchPinAnyRetString(func(v any) string { return dispatchPinArgs(v) }),
			argSets: [][]any{{}, {int64(65)}},
		},
		{
			name:    "func(any, any) string",
			fast:    func(a, b any) string { return dispatchPinArgs(a, b) },
			slow:    dispatchPinAnyAnyRetString(func(a, b any) string { return dispatchPinArgs(a, b) }),
			argSets: [][]any{{int64(1)}, {int64(1), nil}, {"a", "b"}},
		},
		{
			name:    "func(any, any) any",
			fast:    func(a, b any) any { return dispatchPinArgs(a, b) },
			slow:    dispatchPinAnyAnyRetAny(func(a, b any) any { return dispatchPinArgs(a, b) }),
			argSets: [][]any{{}, {"a", int64(2)}},
		},
		{
			name:    "func(string) string",
			fast:    func(s string) string { return "<" + s + ">" },
			slow:    dispatchPinStrRetStr(func(s string) string { return "<" + s + ">" }),
			argSets: [][]any{{}, {int64(65)}, {nil}, {"a"}},
		},
		{
			name:    "func() string",
			fast:    func() string { return "bare" },
			slow:    dispatchPinRetStr(func() string { return "bare" }),
			argSets: [][]any{{}, {"surplus"}},
		},
		{
			name:    "func() any",
			fast:    func() any { return int64(9) },
			slow:    dispatchPinRetAny(func() any { return int64(9) }),
			argSets: [][]any{{}},
		},
		{
			name:    "func(...any) any",
			fast:    func(vs ...any) any { return dispatchPinArgs(vs...) },
			slow:    dispatchPinVarAnyRetAny(func(vs ...any) any { return dispatchPinArgs(vs...) }),
			argSets: [][]any{{}, {nil, int64(3), "s"}},
		},
		{
			name:    "func(...any) bool",
			fast:    func(vs ...any) bool { return len(vs) > 1 },
			slow:    dispatchPinVarAnyRetBool(func(vs ...any) bool { return len(vs) > 1 }),
			argSets: [][]any{{}, {int64(1)}, {int64(1), int64(2)}},
		},
		{
			name:    "func(string) any",
			fast:    func(s string) any { return "<" + s + ">" },
			slow:    dispatchPinStrRetAny(func(s string) any { return "<" + s + ">" }),
			argSets: [][]any{{}, {int64(65)}},
		},
		{
			name:    "func(string) bool",
			fast:    func(s string) bool { return s != "" },
			slow:    dispatchPinStrRetBool(func(s string) bool { return s != "" }),
			argSets: [][]any{{}, {int64(0)}, {"x"}},
		},
		{
			name:    "func(string) int64",
			fast:    func(s string) int64 { return int64(len(s)) },
			slow:    dispatchPinStrRetInt(func(s string) int64 { return int64(len(s)) }),
			argSets: [][]any{{}, {int64(65)}, {"abc"}},
		},
		{
			name:    "func() int64",
			fast:    func() int64 { return 15 },
			slow:    dispatchPinRetInt(func() int64 { return 15 }),
			argSets: [][]any{{}},
		},
		{
			// int64(7) and "7" comparing equal is the coercion pin: both
			// parameters arrive as the string "7" on both paths.
			name:    "func(string, string) bool",
			fast:    func(a, b string) bool { return a == b },
			slow:    dispatchPinStrStrRetBool(func(a, b string) bool { return a == b }),
			argSets: [][]any{{int64(7), "7"}, {"a"}, {}},
		},
		{
			name:    "func(string, string) string",
			fast:    func(a, b string) string { return a + "|" + b },
			slow:    dispatchPinStrStrRetStr(func(a, b string) string { return a + "|" + b }),
			argSets: [][]any{{int64(65), nil}, {"a"}},
		},
		{
			name: "func(string, string) (bool, error)",
			fast: func(a, b string) (bool, error) {
				if a == "boom" {
					return false, dispatchPinErr
				}
				return a == b, nil
			},
			slow: dispatchPinStrStrRetBoolErr(func(a, b string) (bool, error) {
				if a == "boom" {
					return false, dispatchPinErr
				}
				return a == b, nil
			}),
			argSets: [][]any{{"x", "x"}, {"boom"}},
		},
		{
			name: "func(any) int64",
			fast: func(v any) int64 {
				if n, ok := v.(int64); ok {
					return n + 1
				}
				return -1
			},
			slow: dispatchPinAnyRetInt(func(v any) int64 {
				if n, ok := v.(int64); ok {
					return n + 1
				}
				return -1
			}),
			argSets: [][]any{{}, {int64(4)}, {"s"}},
		},
		{
			name: "func(any) float64",
			fast: func(v any) float64 {
				if f, ok := v.(float64); ok {
					return f * 2
				}
				return -1
			},
			slow: dispatchPinAnyRetFloat(func(v any) float64 {
				if f, ok := v.(float64); ok {
					return f * 2
				}
				return -1
			}),
			argSets: [][]any{{1.5}, {nil}},
		},
		{
			name: "func(any) (any, error)",
			fast: func(v any) (any, error) {
				if dispatchPinBoom(v) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(v), nil
			},
			slow: dispatchPinAnyRetAnyErr(func(v any) (any, error) {
				if dispatchPinBoom(v) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(v), nil
			}),
			argSets: [][]any{{}, {"boom"}, {int64(65)}},
		},
		{
			name: "func(any) (bool, error)",
			fast: func(v any) (bool, error) {
				if dispatchPinBoom(v) {
					return false, dispatchPinErr
				}
				return v != nil, nil
			},
			slow: dispatchPinAnyRetBoolErr(func(v any) (bool, error) {
				if dispatchPinBoom(v) {
					return false, dispatchPinErr
				}
				return v != nil, nil
			}),
			argSets: [][]any{{}, {"boom"}, {"x"}},
		},
		{
			name: "func(any, any) (bool, error)",
			fast: func(a, b any) (bool, error) {
				if dispatchPinBoom(a) {
					return false, dispatchPinErr
				}
				return fmt.Sprint(a) == fmt.Sprint(b), nil
			},
			slow: dispatchPinAnyAnyRetBoolErr(func(a, b any) (bool, error) {
				if dispatchPinBoom(a) {
					return false, dispatchPinErr
				}
				return fmt.Sprint(a) == fmt.Sprint(b), nil
			}),
			argSets: [][]any{{"a", "a"}, {"boom", nil}, {"a"}},
		},
		{
			name: "func(any, ...any) (any, error)",
			fast: func(v any, rest ...any) (any, error) {
				if dispatchPinBoom(v) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(append([]any{v}, rest...)...), nil
			},
			slow: dispatchPinAnyVarAnyRetAnyErr(func(v any, rest ...any) (any, error) {
				if dispatchPinBoom(v) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(append([]any{v}, rest...)...), nil
			}),
			argSets: [][]any{{}, {"h", nil, int64(2)}, {"boom"}},
		},
		{
			name: "func(any, ...any) *model.Array",
			fast: func(v any, rest ...any) *model.Array {
				return dispatchPinArray(append([]any{v}, rest...)...)
			},
			slow: dispatchPinAnyVarAnyRetArr(func(v any, rest ...any) *model.Array {
				return dispatchPinArray(append([]any{v}, rest...)...)
			}),
			argSets: [][]any{{}, {"h", int64(1), nil}},
		},
		{
			name: "func(string, ...any) string",
			fast: func(s string, rest ...any) string { return s + "|" + dispatchPinArgs(rest...) },
			slow: dispatchPinStrVarAnyRetStr(func(s string, rest ...any) string {
				return s + "|" + dispatchPinArgs(rest...)
			}),
			argSets: [][]any{{}, {int64(65), "a", nil}},
		},
		{
			// The variadic tail is coerced element by element: int64(1)
			// arrives as "1", nil as "", true as "1", on both paths.
			name: "func(string, ...string) string",
			fast: func(s string, rest ...string) string {
				return strings.Join(append([]string{s}, rest...), "+")
			},
			slow: dispatchPinStrVarStrRetStr(func(s string, rest ...string) string {
				return strings.Join(append([]string{s}, rest...), "+")
			}),
			argSets: [][]any{{}, {"h"}, {"h", int64(1), nil, true}},
		},
		{
			name:    "func() *model.Array",
			fast:    func() *model.Array { return dispatchPinArray("fixed") },
			slow:    dispatchPinRetArr(func() *model.Array { return dispatchPinArray("fixed") }),
			argSets: [][]any{{}},
		},
		{
			name: "func(any, any, ...any) (any, error)",
			fast: func(a, b any, rest ...any) (any, error) {
				if dispatchPinBoom(a) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(append([]any{a, b}, rest...)...), nil
			},
			slow: dispatchPinAnyAnyVarAnyRetAnyErr(func(a, b any, rest ...any) (any, error) {
				if dispatchPinBoom(a) {
					return nil, dispatchPinErr
				}
				return dispatchPinArgs(append([]any{a, b}, rest...)...), nil
			}),
			argSets: [][]any{{}, {"a"}, {"a", int64(2), nil, "z"}, {"boom"}},
		},
		{
			name: "func(any, string) any",
			fast: func(v any, s string) any { return dispatchPinArgs(v) + "/" + s },
			slow: dispatchPinAnyStrRetAny(func(v any, s string) any {
				return dispatchPinArgs(v) + "/" + s
			}),
			argSets: [][]any{{}, {int64(1), int64(65)}, {"a", nil}},
		},
		{
			name: "func(string, any) (any, error)",
			fast: func(s string, v any) (any, error) {
				if s == "boom" {
					return nil, dispatchPinErr
				}
				return s + "/" + dispatchPinArgs(v), nil
			},
			slow: dispatchPinStrAnyRetAnyErr(func(s string, v any) (any, error) {
				if s == "boom" {
					return nil, dispatchPinErr
				}
				return s + "/" + dispatchPinArgs(v), nil
			}),
			argSets: [][]any{{}, {int64(65), "x"}, {"boom"}},
		},
		{
			name: "func(string, any, ...any) (any, error)",
			fast: func(s string, v any, rest ...any) (any, error) {
				if s == "boom" {
					return nil, dispatchPinErr
				}
				return s + "/" + dispatchPinArgs(append([]any{v}, rest...)...), nil
			},
			slow: dispatchPinStrAnyVarAnyRetAnyErr(func(s string, v any, rest ...any) (any, error) {
				if s == "boom" {
					return nil, dispatchPinErr
				}
				return s + "/" + dispatchPinArgs(append([]any{v}, rest...)...), nil
			}),
			argSets: [][]any{{}, {"k", int64(1), nil, "t"}, {"boom"}},
		},
		{
			name: "func(string, string, ...any) (any, error)",
			fast: func(a, b string, rest ...any) (any, error) {
				if a == "boom" {
					return nil, dispatchPinErr
				}
				return a + "|" + b + "|" + dispatchPinArgs(rest...), nil
			},
			slow: dispatchPinStrStrVarAnyRetAnyErr(func(a, b string, rest ...any) (any, error) {
				if a == "boom" {
					return nil, dispatchPinErr
				}
				return a + "|" + b + "|" + dispatchPinArgs(rest...), nil
			}),
			argSets: [][]any{{}, {int64(1), nil, "t", int64(2)}, {"boom"}},
		},
		{
			name: "func(string, string) (any, error)",
			fast: func(a, b string) (any, error) {
				if a == "boom" {
					return nil, dispatchPinErr
				}
				return a + "|" + b, nil
			},
			slow: dispatchPinStrStrRetAnyErr(func(a, b string) (any, error) {
				if a == "boom" {
					return nil, dispatchPinErr
				}
				return a + "|" + b, nil
			}),
			argSets: [][]any{{"a", int64(66)}, {"boom"}, {}},
		},
		{
			// The fast case for this shape only fires when the second
			// argument already is a bool; a missing or int64 second argument
			// falls through to reflection, and both spellings must land the
			// same way (padded false, or the same TypeError).
			name: "func(string, bool) (any, error)",
			fast: func(s string, b bool) (any, error) { return fmt.Sprintf("%s:%v", s, b), nil },
			slow: dispatchPinStrBoolRetAnyErr(func(s string, b bool) (any, error) {
				return fmt.Sprintf("%s:%v", s, b), nil
			}),
			argSets: [][]any{{"x", true}, {"x", false}, {}, {"x", int64(1)}},
		},
		{
			name:    "func(string) func(any)",
			fast:    func(s string) func(any) { return dispatchPinSink },
			slow:    dispatchPinStrRetFunc(func(s string) func(any) { return dispatchPinSink }),
			argSets: [][]any{{}, {"k"}, {int64(65)}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fastType, slowType := reflect.TypeOf(test.fast), reflect.TypeOf(test.slow)
			if fastType == slowType {
				t.Fatalf("slow variant shares the fast variant's dynamic type %v; it would not force the reflect path", fastType)
			}
			if !slowType.ConvertibleTo(fastType) {
				t.Fatalf("slow variant %v does not carry the fast variant's signature %v", slowType, fastType)
			}
			for i, args := range test.argSets {
				// Each path gets its own copy: variadic dispatch aliases the
				// argument slice into the binding.
				fastV, fastErr := invokeAny(test.fast, append([]any(nil), args...))
				slowV, slowErr := invokeAny(test.slow, append([]any(nil), args...))
				if (fastErr == nil) != (slowErr == nil) {
					t.Fatalf("args #%d %v: fast err = %v, reflect err = %v", i, args, fastErr, slowErr)
				}
				if fastErr != nil && fastErr.Error() != slowErr.Error() {
					t.Fatalf("args #%d %v: fast err %q, reflect err %q", i, args, fastErr, slowErr)
				}
				if fastErr != nil {
					// The value beside a non-nil error is not pinned: every
					// dispatch site discards it, and the paths disagree on it
					// today (the fast case hands back the binding's bool, the
					// reflect path hands back nil).
					continue
				}
				if !dispatchPinEqual(fastV, slowV) {
					t.Fatalf("args #%d %v: fast = %#v, reflect = %#v", i, args, fastV, slowV)
				}
			}
		})
	}
}

// A call passing more arguments than the callable declares is refused as an
// ArgumentCountError carrying the declared and passed counts, on both paths:
// the check runs before dispatch. Direct invocation carries no PHP name, so
// Name stays empty and the message says "call()".
func TestDispatchPinArgumentCount(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		args []any
		want int
		got  int
	}{
		{"fast shape", func(v any) any { return v }, []any{int64(1), int64(2)}, 1, 2},
		{"reflect shape", func(a, b, c string) string { return a + b + c }, []any{"a", "b", "c", "d"}, 3, 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := invokeAny(test.fn, test.args)
			var count *ArgumentCountError
			if !errors.As(err, &count) {
				t.Fatalf("err = %v, want *ArgumentCountError", err)
			}
			if count.Want != test.want || count.Got != test.got {
				t.Fatalf("Want/Got = %d/%d, want %d/%d", count.Want, count.Got, test.want, test.got)
			}
			if count.Name != "" {
				t.Fatalf("Name = %q, want empty from direct invocation", count.Name)
			}
			if !strings.Contains(err.Error(), "expects at most") {
				t.Fatalf("err = %v", err)
			}
		})
	}

	// A variadic callable takes any surplus.
	if _, err := invokeAny(func(vs ...any) any { return len(vs) }, []any{1, 2, 3, 4, 5}); err != nil {
		t.Fatalf("variadic surplus: %v", err)
	}
}

// Fewer arguments than declared zero-pads: nil for an any parameter, "" for a
// string parameter, on both paths.
func TestDispatchPinArgumentPadding(t *testing.T) {
	// Fast shape: func(string) string with no arguments.
	v, err := invokeAny(func(s string) string { return "<" + s + ">" }, nil)
	if err != nil {
		t.Fatalf("fast: %v", err)
	}
	if v != "<>" {
		t.Fatalf("fast padded value = %#v, want %q", v, "<>")
	}

	// Reflect shape: three parameters, one argument.
	v, err = invokeAny(func(a string, b any, c string) string {
		return fmt.Sprintf("%q/%v(%T)/%q", a, b, b, c)
	}, []any{"a"})
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	if want := `"a"/<nil>(<nil>)/""`; v != want {
		t.Fatalf("reflect padded value = %#v, want %q", v, want)
	}
}

// The reflect path coerces arguments to the declared parameter types: string
// parameters render the value the way PHP does, time.Duration accepts Go's
// duration syntax, and an unconvertible argument is refused as a TypeError
// naming the position and the PHP spellings of both types.
func TestDispatchPinReflectCoercion(t *testing.T) {
	// Three string parameters keep the signature out of the fast switch.
	v, err := invokeAny(func(a, b, c string) string { return a + "|" + b + "|" + c }, []any{int64(65), 1.5, true})
	if err != nil {
		t.Fatalf("string coercion: %v", err)
	}
	if want := "65|1.5|1"; v != want {
		t.Fatalf("string coercion = %#v, want %q", v, want)
	}

	// A Duration parameter parses a duration string, whitespace tolerated.
	v, err = invokeAny(func(d time.Duration) int64 { return int64(d / time.Minute) }, []any{" 30m "})
	if err != nil {
		t.Fatalf("duration: %v", err)
	}
	if v != int64(30) {
		t.Fatalf("duration = %#v, want 30", v)
	}

	// A string a Duration parameter cannot parse is a TypeError, not a panic.
	_, err = invokeAny(func(d time.Duration) int64 { return 0 }, []any{"not a duration"})
	var mismatch *TypeError
	if !errors.As(err, &mismatch) {
		t.Fatalf("bad duration err = %v, want *TypeError", err)
	}

	// An unconvertible argument reports position, wanted and given types.
	_, err = invokeAny(func(s string, limit int64) string { return s }, []any{"a", model.NewArray()})
	mismatch = nil
	if !errors.As(err, &mismatch) {
		t.Fatalf("err = %v, want *TypeError", err)
	}
	if mismatch.Position != 2 || mismatch.Want != "int" || mismatch.Got != "array" {
		t.Fatalf("TypeError = %+v", mismatch)
	}
	if want := "call(): Argument #2 must be of type int, array given"; err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

// A binding that panics surfaces as a HostPanicError whose Callable spells the
// original function's dynamic type, on both paths: the recover sits in
// invokeAny, above the split.
func TestDispatchPinHostPanic(t *testing.T) {
	tests := []struct {
		name string
		fn   any
	}{
		{"fast path", func(s string) string { panic("exploded") }},
		{"reflect path", dispatchPinStrRetStr(func(s string) string { panic("exploded") })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := invokeAny(test.fn, []any{"x"})
			if v != nil {
				t.Fatalf("value = %#v, want nil", v)
			}
			var hp *HostPanicError
			if !errors.As(err, &hp) {
				t.Fatalf("err = %v, want *HostPanicError", err)
			}
			if want := fmt.Sprintf("%T", test.fn); hp.Callable != want {
				t.Fatalf("Callable = %q, want %q", hp.Callable, want)
			}
			if !strings.Contains(hp.Error(), "exploded") {
				t.Fatalf("err = %v", hp)
			}
		})
	}
}

// wantsContext reads only the first parameter's type.
func TestDispatchPinWantsContext(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		want bool
	}{
		{"context first", func(ctx context.Context, s string) string { return s }, true},
		{"context second", func(s string, ctx context.Context) string { return s }, false},
		{"no context", func(s string) string { return s }, false},
		{"no parameters", func() string { return "" }, false},
		{"not a func", 42, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := wantsContext(reflect.TypeOf(test.fn)); got != test.want {
				t.Fatalf("wantsContext = %v, want %v", got, test.want)
			}
		})
	}
}

// A binding declaring a leading context receives one carrying the scope the
// call ran under; a binding without one is called with the script's arguments
// alone, and a nil scope is fine because no context is ever built for it.
func TestDispatchPinContextInjection(t *testing.T) {
	rt := New(io.Discard, Options{})
	scope := rt.newScope()

	var seen *Scope
	withCtx := func(ctx context.Context, s string) string {
		if sc, ok := ScopeFromContext(ctx); ok {
			seen = sc
		}
		return "<" + s + ">"
	}
	v, err := rt.invokeWithScopeContext(withCtx, []any{"x"}, scope)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v != "<x>" {
		t.Fatalf("value = %#v, want %q", v, "<x>")
	}
	if seen != scope {
		t.Fatalf("binding saw scope %p, want %p", seen, scope)
	}

	// Scope-free binding, nil scope: no context is derived, nothing panics.
	v, err = rt.invokeWithScopeContext(func(s string) string { return s + "!" }, []any{"y"}, nil)
	if err != nil {
		t.Fatalf("nil scope: %v", err)
	}
	if v != "y!" {
		t.Fatalf("value = %#v, want %q", v, "y!")
	}
}

// Receivers for the callGoMethod resolution tests. Two distinct types carry a
// same-named method so per-type caching can be watched for cross-talk, and one
// type declares its method on the pointer only so the pointer and value method
// sets resolve separately.
type dispatchPinReceiver struct{ id string }

func (r *dispatchPinReceiver) Hello() string { return "recv-hello:" + r.id }

func (r *dispatchPinReceiver) GetID() string { return r.id }

func (r *dispatchPinReceiver) Describe(ctx context.Context, s string) string {
	if _, ok := ScopeFromContext(ctx); ok {
		return "ctx:" + s
	}
	return "noctx:" + s
}

func (r *dispatchPinReceiver) Explode() string { panic("kaboom") }

type dispatchPinOther struct{}

func (dispatchPinOther) Hello() string { return "other-hello" }

type dispatchPinPtrOnly struct{}

func (*dispatchPinPtrOnly) Hello() string { return "ptronly-hello" }

// callGoMethod resolves the exact Go name, then case-insensitively, then with
// underscores stripped so a snake_case PHP spelling finds the idiomatic Go
// name. A miss reports PHP's undefined-method message naming the class a
// script sees. Every lookup is repeated so the warm-cache answer is asserted
// to match the cold one, per receiver type.
func TestDispatchPinCallGoMethod(t *testing.T) {
	rt := New(io.Discard, Options{})
	scopeFor := func() *Scope { return rt.newScope() }

	call := func(base any, method string, args ...any) (any, error) {
		return rt.callGoMethod(base, method, args, scopeFor)
	}

	recv := &dispatchPinReceiver{id: "a"}
	tests := []struct {
		name    string
		base    any
		method  string
		args    []any
		want    any
		wantErr string
	}{
		{name: "exact name", base: recv, method: "Hello", want: "recv-hello:a"},
		{name: "case-insensitive", base: recv, method: "hello", want: "recv-hello:a"},
		{name: "snake_case to CamelCase", base: recv, method: "get_id", want: "a"},
		{name: "context auto-injection", base: recv, method: "describe", args: []any{"x"}, want: "ctx:x"},
		{
			name: "miss names the class a script sees", base: recv, method: "nope",
			wantErr: "call to undefined method dispatchPinReceiver::nope()",
		},
		{name: "second type, same method name", base: dispatchPinOther{}, method: "Hello", want: "other-hello"},
		{name: "pointer receiver via pointer", base: &dispatchPinPtrOnly{}, method: "Hello", want: "ptronly-hello"},
		{
			// A method declared on the pointer is not in the value's method
			// set, and the value type caches its own (negative) resolution.
			name: "pointer receiver via value misses", base: dispatchPinPtrOnly{}, method: "Hello",
			wantErr: "call to undefined method dispatchPinPtrOnly::Hello()",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Twice: the first call resolves and caches, the second answers
			// from the cache. Both must agree.
			for pass := 0; pass < 2; pass++ {
				v, err := call(test.base, test.method, test.args...)
				if test.wantErr != "" {
					if err == nil || err.Error() != test.wantErr {
						t.Fatalf("pass %d: err = %v, want %q", pass, err, test.wantErr)
					}
					continue
				}
				if err != nil {
					t.Fatalf("pass %d: %v", pass, err)
				}
				if v != test.want {
					t.Fatalf("pass %d: got %#v, want %#v", pass, v, test.want)
				}
			}
		})
	}
}

// A Go method that panics surfaces as a HostPanicError whose Callable spells
// the receiver's dynamic type and the method name the script used.
func TestDispatchPinCallGoMethodPanic(t *testing.T) {
	rt := New(io.Discard, Options{})
	recv := &dispatchPinReceiver{id: "a"}
	v, err := rt.callGoMethod(recv, "explode", nil, func() *Scope { return rt.newScope() })
	if v != nil {
		t.Fatalf("value = %#v, want nil", v)
	}
	var hp *HostPanicError
	if !errors.As(err, &hp) {
		t.Fatalf("err = %v, want *HostPanicError", err)
	}
	if want := fmt.Sprintf("%T::explode", recv); hp.Callable != want {
		t.Fatalf("Callable = %q, want %q", hp.Callable, want)
	}
}

// adapt wraps any Go callable into the uniform func(...any) (any, error) the
// runtime carries everywhere, and the wrapper answers exactly as a direct
// invocation does, for fast-path and reflect-only signatures alike.
// Runtime.Callable resolves a registered name to the same uniform shape.
func TestDispatchPinUniformABI(t *testing.T) {
	covered := func(s string) string { return "[" + s + "]" }
	uncovered := func(a, b, c string) string { return a + "|" + b + "|" + c }

	tests := []struct {
		name string
		fn   any
		args []any
	}{
		{"covered signature", covered, []any{int64(65)}},
		{"uncovered signature", uncovered, []any{"a", int64(2)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directV, directErr := invokeAny(test.fn, append([]any(nil), test.args...))
			adaptedV, adaptedErr := adapt(test.fn)(append([]any(nil), test.args...)...)
			if (directErr == nil) != (adaptedErr == nil) {
				t.Fatalf("direct err = %v, adapted err = %v", directErr, adaptedErr)
			}
			if !dispatchPinEqual(directV, adaptedV) {
				t.Fatalf("direct = %#v, adapted = %#v", directV, adaptedV)
			}
		})
	}

	// A registered name resolves through Runtime.Callable to the same answer.
	rt := New(io.Discard, Options{})
	rt.RegisterFunc("dispatch_pin_probe", covered)
	call, ok := rt.Callable("dispatch_pin_probe")
	if !ok {
		t.Fatal("Callable did not resolve a registered name")
	}
	v, err := call(int64(65))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if v != "[65]" {
		t.Fatalf("got %#v, want %q", v, "[65]")
	}

	// A non-callable value is refused with the type it actually was.
	if _, err := invokeAny(42, nil); err == nil || !strings.Contains(err.Error(), "not callable: int") {
		t.Fatalf("err = %v, want the not-callable report", err)
	}
}

// dispatchPinRun parses and runs src on rt and returns what it printed.
func dispatchPinRun(t *testing.T, rt *Runtime, out *strings.Builder, src string) string {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out.Reset()
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// The evaluation-environment helpers stay wired whatever the dispatch
// internals become: a method call (__call), a property read (__get), an
// assignment used as an expression (__set), a dynamic function-name call, the
// namespaced global fallback (__func) and func_get_args all behave through
// the public API. The script uses no standard library: the runner's own
// helpers and one registered binding are the whole surface under test.
func TestDispatchPinEnvHelperSmoke(t *testing.T) {
	var out strings.Builder
	rt := New(&out, Options{})
	rt.RegisterFunc("probe", func(s string) string { return "[" + s + "]" })

	src := `<?php
class Greeter {
    public $name = "world";
    function hello($prefix) { return $prefix . " " . $this->name; }
}
function tally() {
    $n = 0;
    foreach (func_get_args() as $arg) { $n = $n + 1; }
    return $n;
}
$g = new Greeter();
echo $g->hello("hi"), "\n";
echo $g->name, "\n";
echo ($x = "assigned"), "\n";
$f = "tally";
echo $f(1, 2, 3), "\n";
echo probe(65), "\n";
`
	want := "hi world\nworld\nassigned\n3\n[65]\n"
	if got := dispatchPinRun(t, rt, &out, src); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	// Inside a namespace an unqualified call falls back to the global name,
	// which is the __func dispatch with its fallback argument. A namespaced
	// file may only declare, so the call sits in a function invoked from a
	// second, namespace-free program on the same runtime.
	declare := "<?php\nnamespace App;\nfunction caller() { return probe(66); }\n"
	if got := dispatchPinRun(t, rt, &out, declare); got != "" {
		t.Fatalf("declaring program printed %q", got)
	}
	nsWant := "[66]\n"
	if got := dispatchPinRun(t, rt, &out, "<?php echo \\App\\caller(), \"\\n\";"); got != nsWant {
		t.Fatalf("namespaced fallback: got %q, want %q", got, nsWant)
	}
}
