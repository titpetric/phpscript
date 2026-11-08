package model_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// check runs one Valid and matches the error against what the case expects: an
// empty want is a value that passes.
func check(t *testing.T, label string, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Errorf("%s: Valid() = %v, want nil", label, err)
	case want != "" && err == nil:
		t.Errorf("%s: Valid() = nil, want an error naming %q", label, want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Errorf("%s: Valid() = %v, want it to name %q", label, err, want)
	}
}

// TestParam_Valid covers a parameter a host filled: it names something, and a
// variadic one declares no default, which php has no syntax for.
func TestParam_Valid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		param model.Param
		want  string
	}{
		{"a plain parameter", model.Param{Name: "id"}, ""},
		{"a default", model.Param{Name: "id", Default: &model.Lit{Value: int64(0)}}, ""},
		{"a variadic", model.Param{Name: "rest", Variadic: true}, ""},
		{"no name", model.Param{}, "declares no name"},
		{"a variadic with a default", model.Param{Name: "rest", Variadic: true, Default: &model.Lit{Value: int64(0)}}, "variadic and declares a default"},
	} {
		check(t, tc.name, tc.param.Valid(), tc.want)
	}
}

// TestField_Valid covers a property declaration: a name, and a visibility php
// spells. The empty spelling is the one an undecorated property holds.
func TestField_Valid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		field model.Field
		want  string
	}{
		{"no visibility", model.Field{Name: "id"}, ""},
		{"public", model.Field{Name: "id", Visibility: "public"}, ""},
		{"protected", model.Field{Name: "id", Visibility: "protected"}, ""},
		{"private", model.Field{Name: "id", Visibility: "private"}, ""},
		{"no name", model.Field{}, "declares no name"},
		{"a visibility php has no word for", model.Field{Name: "id", Visibility: "internal"}, `declares visibility "internal"`},
	} {
		check(t, tc.name, tc.field.Valid(), tc.want)
	}
}

// TestFuncDecl_Valid covers a function declaration: a name, no parameter
// declared twice, the variadic one last, and a body matching abstract.
//
// The label names the class where the declaration declares one. The error then
// identifies the method.
func TestFuncDecl_Valid(t *testing.T) {
	t.Parallel()
	body := []model.Stmt{&model.Return{}}
	for _, tc := range []struct {
		name string
		decl *model.FuncDecl
		want string
	}{
		{"a free function", &model.FuncDecl{Name: "f", Body: body}, ""},
		{"a method", &model.FuncDecl{Class: "C", Name: "m", Body: body}, ""},
		{"an abstract method", &model.FuncDecl{Class: "C", Name: "m", Abstract: true}, ""},
		{"a variadic last", &model.FuncDecl{Name: "f", Params: []model.Param{{Name: "a"}, {Name: "rest", Variadic: true}}}, ""},
		{"nothing", nil, "nil declaration"},
		{"no name", &model.FuncDecl{}, "declares no name"},
		{"an abstract method with a body", &model.FuncDecl{Class: "C", Name: "m", Abstract: true, Body: body}, "C::m is abstract and declares a body"},
		{"a parameter twice", &model.FuncDecl{Name: "f", Params: []model.Param{{Name: "a"}, {Name: "a"}}}, "declares $a twice"},
		{"a variadic ahead of another", &model.FuncDecl{Name: "f", Params: []model.Param{{Name: "rest", Variadic: true}, {Name: "a"}}}, "variadic ahead of another parameter"},
		{"a parameter with no name", &model.FuncDecl{Name: "f", Params: []model.Param{{}}}, "function f: parameter"},
	} {
		check(t, tc.name, tc.decl.Valid(), tc.want)
	}
}

// TestClassDecl_Valid covers a class declaration: a name, and no member name
// repeated inside its own group.
//
// A property and a constant may share a name, because php reads `$c->x` and
// `C::X` through separate tables. Two methods may not, and the comparison
// folds case the way php's own method lookup folds it.
func TestClassDecl_Valid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		decl *model.ClassDecl
		want string
	}{
		{"a class", &model.ClassDecl{Name: "C", Fields: []model.Field{{Name: "a"}}}, ""},
		{"a property and a constant sharing a name", &model.ClassDecl{
			Name:   "C",
			Fields: []model.Field{{Name: "x"}},
			Consts: []model.Field{{Name: "x"}},
		}, ""},
		{"nothing", nil, "nil declaration"},
		{"no name", &model.ClassDecl{}, "declares no name"},
		{"a property twice", &model.ClassDecl{Name: "C", Fields: []model.Field{{Name: "a"}, {Name: "a"}}}, "declares property $a twice"},
		{"a static twice", &model.ClassDecl{Name: "C", Statics: []model.Field{{Name: "a"}, {Name: "a"}}}, "declares static property $a twice"},
		{"a constant twice", &model.ClassDecl{Name: "C", Consts: []model.Field{{Name: "A"}, {Name: "A"}}}, "declares constant $A twice"},
		{"a method twice", &model.ClassDecl{Name: "C", Methods: []*model.FuncDecl{{Name: "m"}, {Name: "M"}}}, "declares method M() twice"},
		{"a method with no name", &model.ClassDecl{Name: "C", Methods: []*model.FuncDecl{{}}}, "class C: function"},
	} {
		check(t, tc.name, tc.decl.Valid(), tc.want)
	}
}

// TestClass_Valid covers the resolved form the runtime holds: a name, no nil
// method table entry, and no property declared twice.
func TestClass_Valid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		class *model.Class
		want  string
	}{
		{"a class", &model.Class{Name: "C", Methods: map[string]*model.FuncDecl{"m": {Name: "m"}}}, ""},
		{"stdClass", model.StdClass(), ""},
		{"nothing", nil, "class: nil"},
		{"no name", &model.Class{}, "declares no name"},
		{"a nil method", &model.Class{Name: "C", Methods: map[string]*model.FuncDecl{"m": nil}}, "nil declaration under method m()"},
		{"a property twice", &model.Class{Name: "C", Fields: []model.Field{{Name: "a"}, {Name: "a"}}}, "declares property $a twice"},
	} {
		check(t, tc.name, tc.class.Valid(), tc.want)
	}
}

// TestFunc_Valid covers the callable holding exactly one of its two forms: a
// php declaration, or a go func the host registered.
func TestFunc_Valid(t *testing.T) {
	t.Parallel()
	decl := &model.FuncDecl{Name: "f"}
	for _, tc := range []struct {
		name string
		fn   model.Func
		want string
	}{
		{"a declaration", model.Func{Decl: decl}, ""},
		{"a go func", model.Func{Go: func() {}}, ""},
		{"neither", model.Func{}, "neither a declaration nor a go func"},
		{"both", model.Func{Decl: decl, Go: func() {}}, "both a declaration and a go func"},
		{"a declaration with no name", model.Func{Decl: &model.FuncDecl{}}, "declares no name"},
	} {
		check(t, tc.name, tc.fn.Valid(), tc.want)
	}
}

// TestObject_Valid covers the instance: it points at a class, which is the one
// path to its methods and its property order.
func TestObject_Valid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		obj  *model.Object
		want string
	}{
		{"an instance", model.NewObject(&model.Class{Name: "C"}), ""},
		{"a stdClass instance", model.NewStdClass(), ""},
		{"nothing", nil, "object: nil"},
		{"no class", &model.Object{}, "holds no class"},
		{"a class with no name", model.NewObject(&model.Class{}), "declares no name"},
	} {
		check(t, tc.name, tc.obj.Valid(), tc.want)
	}
}
