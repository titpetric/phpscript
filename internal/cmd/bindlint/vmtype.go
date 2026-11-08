package main

import (
	"go/ast"
	"strings"
)

// vmTypes are the runtime's own value types. A script value reaches a binding
// as one of these only when the interpreter built it; the same value built by
// another binding arrives as a Go slice, map or struct.
var vmTypes = map[string]bool{"Array": true, "Object": true}

// paramTypes are the runtime value types and the two interfaces only a runtime
// value satisfies. nativeShape holds the other half a parameter may not take.
var paramTypes = map[string]bool{"Array": true, "Object": true, "Collection": true, "Keyed": true}

// vmType names the runtime value type an expression denotes, pointer, slice,
// map and variadic wrappers included, and "" for every other type.
//
// It reads the spelling and never a type. The qualifier has to be `model`,
// the name every file in the tree imports the package under, and a local
// alias of the type hides it. A rename hides the type from a reader as well,
// and the type checker is the price of seeing through either.
func vmType(e ast.Expr) string {
	return modelType(e, vmTypes)
}

// paramType is vmType over the types a parameter may not declare.
func paramType(e ast.Expr) string {
	return modelType(e, paramTypes)
}

// modelType names the model type an expression denotes, out of the set.
func modelType(e ast.Expr, set map[string]bool) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return modelType(t.X, set)
	case *ast.Ellipsis:
		return modelType(t.Elt, set)
	case *ast.ArrayType:
		return modelType(t.Elt, set)
	case *ast.MapType:
		if name := modelType(t.Key, set); name != "" {
			return name
		}
		return modelType(t.Value, set)
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		if !ok || pkg.Name != "model" || !set[t.Sel.Name] {
			return ""
		}
		return "model." + t.Sel.Name
	}
	return ""
}

// nativeShape reports whether an expression names a Go-native collection: the
// shape a value takes when a binding, and not the interpreter, built it. A
// type switch that has a case for one of these has already routed the native
// value somewhere, and its *model.Array case is the interpreter's half of the
// same decision.
//
// A `[]byte` is none. It holds a php string, and a case for one answers for
// no collection. See phpval.Bytes and modelutil.IsCollection, which agree.
func nativeShape(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.MapType:
		return true
	case *ast.ArrayType:
		elem, ok := t.Elt.(*ast.Ident)
		return !ok || elem.Name != "byte"
	}
	return false
}

// nativeReaders are the calls that read a value whatever shape it has.
// A function that makes one of them has a path for the native value, and the
// type it narrows to is a fast path over it and no refusal of it.
var nativeReaders = map[string]bool{
	"IsCollection": true,
	"RangeValues":  true,
	"LenValues":    true,
	"Values":       true,
	"ValueOf":      true,
	"TypeOf":       true,
}

// vmCapabilities are the methods a Go-native value cannot answer: in-place
// mutation, which a slice behind an interface cannot do, and the typed storage
// `model.Array` keeps for a list of integers. A narrowing that calls one of
// them needs the concrete type and is no finding.
var vmCapabilities = map[string]bool{
	"Append":           true,
	"Set":              true,
	"SetProp":          true,
	"Clear":            true,
	"Delete":           true,
	"DeleteProp":       true,
	"SetID":            true,
	"Int64List":        true,
	"ReplaceInt64List": true,
	"PromoteToMap":     true,
}

// calledName is the function or method name a call expression names, without
// its qualifier: `modelutil.RangeValues(...)` is RangeValues and `a.Append(v)` is
// Append. A call of anything but a name answers "".
func calledName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// receiverName is the variable a method call reads: `options.Get("path")` is
// options. A call on anything but a plain name answers "". A package
// qualifier reads as a receiver and costs no finding, because a receiver
// answers against model.Array's own method names.
func receiverName(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// phpName renders a registered name the way a script types it: the Go source
// escapes a namespace separator and `Runtime\\Router` is one backslash.
func phpName(name string) string {
	return strings.ReplaceAll(name, `\\`, `\`)
}
