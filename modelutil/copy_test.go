package modelutil

import (
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestCopyValue covers PHP's assignment semantics. An array is a value and a
// copy is independent of its source. The copy reaches nested arrays because
// they are values too: a `$copy["a"]["b"] = 1` never shows through the
// original.
//
// The copy stops at objects, which PHP holds by handle, and at the native Go
// collections a binding returns, which belong to the host.
func TestCopyValue(t *testing.T) {
	t.Parallel()
	nested := model.NewArray()
	nested.Set("b", int64(1))
	source := model.NewArray()
	source.Set("a", nested)

	copied, ok := CopyValue(source).(*model.Array)
	if !ok {
		t.Fatal("CopyValue on an *model.Array answered another type")
	}
	if copied == source {
		t.Fatal("CopyValue answered the array it was given")
	}

	copied.Set("a2", int64(9))
	if _, ok := source.Get("a2"); ok {
		t.Error("a key written on the copy is readable through the source")
	}

	inner, _ := copied.Get("a")
	if inner.(*model.Array) == nested {
		t.Fatal("the nested array is shared with the source")
	}
	inner.(*model.Array).Set("b", int64(2))
	if got, _ := nested.Get("b"); got != int64(1) {
		t.Errorf("the source's nested array holds %v after a write through the copy, want 1", got)
	}

	obj := model.NewStdClass()
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
// array answers nil and a caller holding one needs no guard.
func TestCopyArray(t *testing.T) {
	t.Parallel()
	if got := CopyArray(nil); got != nil {
		t.Errorf("CopyArray(nil) = %v, want nil", got)
	}

	source := model.NewArray()
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

// TestRootName covers the name an lvalue roots at. An index and a property
// chain root at the variable they start from, because the storage they reach
// lives inside the value that variable holds. Parentheses are transparent.
//
// A bare identifier with Const set is a constant and no variable and it roots
// at no variable: a constant is not assignable, and treating `PHP_EOL` as the name
// "PHP_EOL" would match a loop variable spelled the same.
func TestRootName(t *testing.T) {
	t.Parallel()
	v := &model.Var{Name: "v"}

	for _, tc := range []struct {
		name string
		expr model.Expr
		want string
	}{
		{"variable", v, "v"},
		{"index", &model.Index{Base: v, Index: &model.Lit{Value: "k"}}, "v"},
		{"nested index", &model.Index{Base: &model.Index{Base: v, Index: &model.Lit{Value: "k"}}, Index: &model.Lit{Value: int64(0)}}, "v"},
		{"property", &model.PropAccess{Base: v, Name: "p"}, "v"},
		{"property chain", &model.PropAccess{Base: &model.PropAccess{Base: v, Name: "p"}, Name: "q"}, "v"},
		{"index of a property", &model.Index{Base: &model.PropAccess{Base: v, Name: "p"}, Index: &model.Lit{Value: "k"}}, "v"},
		{"parenthesized", &model.Parenthesized{X: v}, "v"},
		{"index of a parenthesized base", &model.Index{Base: &model.Parenthesized{X: v}, Index: &model.Lit{Value: "k"}}, "v"},
		{"constant", &model.Var{Name: "PHP_EOL", Const: true}, ""},
		{"literal", &model.Lit{Value: int64(1)}, ""},
		{"call", &model.Call{Name: "f"}, ""},
		{"nothing", nil, ""},
	} {
		if got := RootName(tc.expr); got != tc.want {
			t.Errorf("%s: RootName() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestAssignsTo covers the question a by-value foreach compiles against: would
// the body notice the copy. phpscript's arrays are pointers and a by-value
// loop copies up front, and copying every element of every loop would be a
// large price for a semantic almost no loop uses.
//
// The test is on the root name and no exact shape. A write to `$v["k"]` counts,
// because the element it reaches lives inside the value the loop variable
// holds. Over-reporting costs a copy nothing observes; under-reporting would
// let a by-value loop edit its source and the undecidable cases answer true: a
// target with no name to root on, and an include, which shares this scope and
// is not visible from here.
func TestAssignsTo(t *testing.T) {
	t.Parallel()
	v := &model.Var{Name: "v"}
	other := &model.Var{Name: "other"}
	one := &model.Lit{Value: int64(1)}

	for _, tc := range []struct {
		name string
		body []model.Stmt
		want bool
	}{
		{"assignment", []model.Stmt{&model.Assign{Target: v, Op: "=", Value: one}}, true},
		{"append", []model.Stmt{&model.Assign{Target: v, Op: "[]=", Value: one}}, true},
		{"element", []model.Stmt{&model.Assign{Target: &model.Index{Base: v, Index: &model.Lit{Value: "k"}}, Op: "=", Value: one}}, true},
		{"property", []model.Stmt{&model.Assign{Target: &model.PropAccess{Base: v, Name: "p"}, Op: "=", Value: one}}, true},
		{"increment", []model.Stmt{&model.ExprStmt{X: &model.Unary{Op: "++", X: v, Postfix: true}}}, true},
		{"decrement", []model.Stmt{&model.ExprStmt{X: &model.Unary{Op: "--", X: v}}}, true},
		{"destructuring", []model.Stmt{&model.Assign{Target: &model.ListExpr{Elems: []model.Expr{nil, v}}, Op: "=", Value: other}}, true},
		{"assignment in a condition", []model.Stmt{&model.If{Cond: &model.AssignExpr{Target: v, Op: "=", Value: one}}}, true},
		{"unset", []model.Stmt{&model.Unset{Targets: []model.Expr{v}}}, true},
		{"output parameter", []model.Stmt{&model.ExprStmt{X: &model.Call{Name: "preg_match", Args: []model.Expr{one, one, v}}}}, true},
		{"a nested loop rebinding the name", []model.Stmt{&model.Foreach{Source: other, ValTarget: v}}, true},
		{"a nested loop rebinding through the deprecated field", []model.Stmt{&model.Foreach{Source: other, ValVar: "v"}}, true},
		{"a catch clause binding the name", []model.Stmt{&model.Try{Catches: []model.Catch{{Type: "Exception", Var: "v"}}}}, true},
		{"a write inside a switch arm", []model.Stmt{&model.Switch{
			Cond:  other,
			Cases: []model.SwitchCase{{Value: one, Body: []model.Stmt{&model.Assign{Target: v, Op: "=", Value: one}}}},
		}}, true},
		{"a write inside a loop", []model.Stmt{&model.For{Body: []model.Stmt{&model.Assign{Target: v, Op: "=", Value: one}}}}, true},
		{"include", []model.Stmt{&model.ExprStmt{X: &model.Include{Keyword: "include", Path: &model.Lit{Value: "f.php"}}}}, true},

		{"no statements", nil, false},
		{"a read", []model.Stmt{&model.Echo{Args: []model.Expr{v}}}, false},
		{"a read on the right", []model.Stmt{&model.Assign{Target: other, Op: "=", Value: v}}, false},
		{"a read as an index", []model.Stmt{&model.Assign{Target: &model.Index{Base: other, Index: v}, Op: "=", Value: one}}, false},
		{"a read through a property", []model.Stmt{&model.Echo{Args: []model.Expr{&model.PropAccess{Base: v, Name: "p"}}}}, false},
		{"a negation", []model.Stmt{&model.ExprStmt{X: &model.Unary{Op: "!", X: v}}}, false},
		{"another name", []model.Stmt{&model.Assign{Target: other, Op: "=", Value: one}}, false},
		{"a closure body", []model.Stmt{&model.ExprStmt{X: &model.Closure{Body: []model.Stmt{&model.Assign{Target: v, Op: "=", Value: one}}}}}, false},
		{"a write to a by-reference capture", []model.Stmt{&model.ExprStmt{X: &model.Closure{
			Uses: []model.ClosureUse{{Name: "v", ByRef: true}},
			Body: []model.Stmt{&model.Assign{Target: v, Op: "+=", Value: one}},
		}}}, false},
	} {
		if got := AssignsTo(v, tc.body); got != tc.want {
			t.Errorf("%s: AssignsTo() = %v, want %v", tc.name, got, tc.want)
		}
	}

	if !AssignsTo(one, []model.Stmt{&model.Echo{Args: []model.Expr{one}}}) {
		t.Error("AssignsTo() = false for a target with no name to root on, want true")
	}
}
