package model

import (
	"reflect"
	"testing"
)

// TestCopyValue covers PHP's assignment semantics. An array is a value, so a
// copy is independent of its source, and the copy reaches nested arrays because
// they are values too: `$copy["a"]["b"] = 1` cannot be observed through the
// original.
//
// The copy stops at objects, which PHP holds by handle, and at the native Go
// collections a binding returns, which belong to the host.
func TestCopyValue(t *testing.T) {
	nested := NewArray()
	nested.Set("b", int64(1))
	source := NewArray()
	source.Set("a", nested)

	copied, ok := CopyValue(source).(*Array)
	if !ok {
		t.Fatal("CopyValue on an *Array answered another type")
	}
	if copied == source {
		t.Fatal("CopyValue answered the array it was given")
	}

	copied.Set("a2", int64(9))
	if _, ok := source.Get("a2"); ok {
		t.Error("a key written on the copy is readable through the source")
	}

	inner, _ := copied.Get("a")
	if inner.(*Array) == nested {
		t.Fatal("the nested array is shared with the source")
	}
	inner.(*Array).Set("b", int64(2))
	if got, _ := nested.Get("b"); got != int64(1) {
		t.Errorf("the source's nested array holds %v after a write through the copy, want 1", got)
	}

	obj := NewStdClass()
	if CopyValue(obj) != obj {
		t.Error("CopyValue copied an object, which PHP holds by handle")
	}
	if got := CopyValue("text"); got != "text" {
		t.Errorf("CopyValue(%q) = %v", "text", got)
	}
	if got := CopyValue(nil); got != nil {
		t.Errorf("CopyValue(nil) = %v, want nil", got)
	}

	native := []string{"a"}
	passed, ok := CopyValue(native).([]string)
	if !ok {
		t.Fatal("CopyValue on a []string answered another type")
	}
	native[0] = "b"
	if passed[0] != "b" {
		t.Error("CopyValue copied a native Go slice, which belongs to the host")
	}
}

// TestCopyArray covers the shape the copy comes out in: the same keys in the
// same order, because insertion order is part of a PHP array's value. A nil
// array answers nil, so a caller holding one needs no guard of its own.
func TestCopyArray(t *testing.T) {
	if got := CopyArray(nil); got != nil {
		t.Errorf("CopyArray(nil) = %v, want nil", got)
	}

	source := NewArray()
	source.Append("first")
	source.Append("second")
	source.Set("k", "v")

	out := CopyArray(source)
	if got, want := out.Len(), source.Len(); got != want {
		t.Fatalf("the copy holds %d entries, want %d", got, want)
	}
	if got, want := out.Keys(), []any{int64(0), int64(1), "k"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the copy's keys are %v, want %v", got, want)
	}
	source.Range(func(key, val any) bool {
		got, ok := out.Get(key)
		if !ok {
			t.Errorf("the copy holds nothing under %v", key)
			return true
		}
		if got != val {
			t.Errorf("the copy holds %v under %v, want %v", got, key, val)
		}
		return true
	})
}

// TestRootName covers the name an lvalue is rooted at. An index and a property
// chain are rooted at the variable they start from, because the storage they
// reach lives inside the value that variable holds. Parentheses are
// transparent.
//
// A bare identifier carrying Const is a constant and no variable, so it roots
// at nothing: a constant is not assignable, and treating `PHP_EOL` as the name
// "PHP_EOL" would match a loop variable spelled the same.
func TestRootName(t *testing.T) {
	v := &Var{Name: "v"}

	for _, tc := range []struct {
		name string
		expr Expr
		want string
	}{
		{"variable", v, "v"},
		{"index", &Index{Base: v, Index: &Lit{Value: "k"}}, "v"},
		{"nested index", &Index{Base: &Index{Base: v, Index: &Lit{Value: "k"}}, Index: &Lit{Value: int64(0)}}, "v"},
		{"property", &PropAccess{Base: v, Name: "p"}, "v"},
		{"property chain", &PropAccess{Base: &PropAccess{Base: v, Name: "p"}, Name: "q"}, "v"},
		{"index of a property", &Index{Base: &PropAccess{Base: v, Name: "p"}, Index: &Lit{Value: "k"}}, "v"},
		{"parenthesized", &Parenthesized{X: v}, "v"},
		{"index of a parenthesized base", &Index{Base: &Parenthesized{X: v}, Index: &Lit{Value: "k"}}, "v"},
		{"constant", &Var{Name: "PHP_EOL", Const: true}, ""},
		{"literal", &Lit{Value: int64(1)}, ""},
		{"call", &Call{Name: "f"}, ""},
		{"nothing", nil, ""},
	} {
		if got := RootName(tc.expr); got != tc.want {
			t.Errorf("%s: RootName() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestAssignsTo covers the question a by-value foreach is compiled against:
// would the body notice the copy. phpscript's arrays are pointers, so the copy
// is made up front, and copying every element of every loop would be a large
// price for a semantic almost no loop uses.
//
// The test is on the root name and no exact shape. A write to `$v["k"]` counts,
// because the element it reaches lives inside the value the loop variable
// holds. Over-reporting costs a copy nothing observes; under-reporting would
// let a by-value loop edit its source, so the undecidable cases answer true: a
// target with no name to root on, and an include, which shares this scope and
// is not visible from here.
func TestAssignsTo(t *testing.T) {
	v := &Var{Name: "v"}
	other := &Var{Name: "other"}
	one := &Lit{Value: int64(1)}

	for _, tc := range []struct {
		name string
		body []Stmt
		want bool
	}{
		{"assignment", []Stmt{&Assign{Target: v, Op: "=", Value: one}}, true},
		{"append", []Stmt{&Assign{Target: v, Op: "[]=", Value: one}}, true},
		{"element", []Stmt{&Assign{Target: &Index{Base: v, Index: &Lit{Value: "k"}}, Op: "=", Value: one}}, true},
		{"property", []Stmt{&Assign{Target: &PropAccess{Base: v, Name: "p"}, Op: "=", Value: one}}, true},
		{"increment", []Stmt{&ExprStmt{X: &Unary{Op: "++", X: v, Postfix: true}}}, true},
		{"decrement", []Stmt{&ExprStmt{X: &Unary{Op: "--", X: v}}}, true},
		{"destructuring", []Stmt{&Assign{Target: &ListExpr{Elems: []Expr{nil, v}}, Op: "=", Value: other}}, true},
		{"assignment in a condition", []Stmt{&If{Cond: &AssignExpr{Target: v, Op: "=", Value: one}}}, true},
		{"unset", []Stmt{&Unset{Targets: []Expr{v}}}, true},
		{"output parameter", []Stmt{&ExprStmt{X: &Call{Name: "preg_match", Args: []Expr{one, one, v}}}}, true},
		{"a nested loop rebinding the name", []Stmt{&Foreach{Source: other, ValTarget: v}}, true},
		{"a nested loop rebinding through the deprecated field", []Stmt{&Foreach{Source: other, ValVar: "v"}}, true},
		{"a catch clause binding the name", []Stmt{&Try{Catches: []Catch{{Type: "Exception", Var: "v"}}}}, true},
		{"a write inside a switch arm", []Stmt{&Switch{
			Cond:  other,
			Cases: []SwitchCase{{Value: one, Body: []Stmt{&Assign{Target: v, Op: "=", Value: one}}}},
		}}, true},
		{"a write inside a loop", []Stmt{&For{Body: []Stmt{&Assign{Target: v, Op: "=", Value: one}}}}, true},
		{"include", []Stmt{&ExprStmt{X: &Include{Keyword: "include", Path: &Lit{Value: "f.php"}}}}, true},

		{"no statements", nil, false},
		{"a read", []Stmt{&Echo{Args: []Expr{v}}}, false},
		{"a read on the right", []Stmt{&Assign{Target: other, Op: "=", Value: v}}, false},
		{"a read as an index", []Stmt{&Assign{Target: &Index{Base: other, Index: v}, Op: "=", Value: one}}, false},
		{"a read through a property", []Stmt{&Echo{Args: []Expr{&PropAccess{Base: v, Name: "p"}}}}, false},
		{"a negation", []Stmt{&ExprStmt{X: &Unary{Op: "!", X: v}}}, false},
		{"another name", []Stmt{&Assign{Target: other, Op: "=", Value: one}}, false},
		{"a closure body", []Stmt{&ExprStmt{X: &Closure{Body: []Stmt{&Assign{Target: v, Op: "=", Value: one}}}}}, false},
	} {
		if got := AssignsTo(v, tc.body); got != tc.want {
			t.Errorf("%s: AssignsTo() = %v, want %v", tc.name, got, tc.want)
		}
	}

	if !AssignsTo(one, []Stmt{&Echo{Args: []Expr{one}}}) {
		t.Error("AssignsTo() = false for a target with no name to root on, want true")
	}
}
