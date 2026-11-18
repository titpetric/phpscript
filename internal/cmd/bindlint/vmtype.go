package main

import (
	"go/ast"
	"strings"
)

// vmTypes are the runtime value types. A script value reaches a binding in one
// of these shapes only when the interpreter built the value. Another binding
// builds the same value as a Go slice, map or struct.
var vmTypes = map[string]bool{"Array": true, "Object": true}

// paramTypes are the runtime value types and the two interfaces only a runtime
// value satisfies. nativeShape holds the other half a parameter may not take.
var paramTypes = map[string]bool{"Array": true, "Object": true, "Collection": true, "Keyed": true}

// vmType names the runtime value type an expression denotes, pointer, slice,
// map and variadic wrappers included, and "" for any other type.
//
// It reads the spelling alone. The qualifier must be `model`. Every file in
// the tree imports the package under that name. A local alias of the type
// hides the type. A rename hides the type from a reader as well. The type
// checker is the price of seeing through either.
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
// shape a value takes when a binding built the value. An interpreter-built
// value is a *model.Array. A type switch with a case for such a shape has already routed
// the native value
// somewhere. Its *model.Array case is the interpreter's half of the same
// decision.
//
// A `[]byte` is none. It holds a php string. A `[]byte` case matches no
// collection. See phpval.Bytes and modelutil.IsCollection, which agree.
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

// nativeReaders are the calls that read a value in any shape. A function that
// calls one has a path for the native value. The type it narrows to is a fast
// path over the value. The narrowing refuses nothing.
var nativeReaders = map[string]bool{
	"IsCollection": true,
	"RangeValues":  true,
	"LenValues":    true,
	"Values":       true,
	"ValueOf":      true,
	"TypeOf":       true,
}

// vmCapabilities are the methods a Go-native value cannot answer: in-place
// mutation (a slice behind an interface cannot do that), plus the typed
// storage `model.Array` keeps for a list of integers. A narrowing that calls such a
// method needs the concrete type and is no finding.
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

// calledName is the function or method name a call expression names, minus
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
// qualifier reads as a receiver and costs no finding. A receiver answers
// against model.Array's method names.
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

// phpName renders a registered name in the form a script types: the Go source
// escapes a namespace separator. `Runtime\\Router` is a single backslash.
func phpName(name string) string {
	return strings.ReplaceAll(name, `\\`, `\`)
}
