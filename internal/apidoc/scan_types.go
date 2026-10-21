package apidoc

import (
	"go/ast"
	"strings"
)

// signature converts an ast function type to PHP parameters and result type
// names. A leading context.Context is the runtime's injection and does not
// appear to PHP.
func signature(ft *ast.FuncType) ([]Param, []string) {
	params := []Param{}
	if ft.Params != nil {
		for _, field := range ft.Params.List {
			if isContextContext(field.Type) {
				continue
			}
			names := []string{""}
			if len(field.Names) > 0 {
				names = names[:0]
				for _, ident := range field.Names {
					names = append(names, ident.Name)
				}
			}
			for _, name := range names {
				params = append(params, astParam(name, field.Type))
			}
		}
	}
	results := []string{}
	if ft.Results != nil {
		for _, field := range ft.Results.List {
			n := len(field.Names)
			if n == 0 {
				n = 1
			}
			for i := 0; i < n; i++ {
				results = append(results, phpTypeAST(field.Type))
			}
		}
	}
	return dedupeParams(params), results
}

// astParam converts one declared parameter. A func(any) parameter is the
// runner's by-reference setter and renders as &$name; any other function
// type is a callable.
func astParam(name string, t ast.Expr) Param {
	p := Param{Name: paramName(name)}
	if ell, ok := t.(*ast.Ellipsis); ok {
		p.Variadic = true
		t = ell.Elt
	}
	if ft, ok := t.(*ast.FuncType); ok {
		if isRefSetter(ft) {
			p.ByRef = true
			return p
		}
		p.Type = "callable"
		return p
	}
	p.Type = phpTypeAST(t)
	if p.Type == "mixed" && isCallbackName(name) {
		p.Type = "callable"
	}
	return p
}

// paramName converts a Go parameter name to the PHP spelling, and names the
// blank identifier for the value a binding takes and ignores.
func paramName(name string) string {
	if name == "" || name == "_" {
		return "unused"
	}
	return camelToSnake(name)
}

// isCallbackName reports whether an untyped parameter's name marks it as a
// callable checked at call time.
func isCallbackName(name string) bool {
	switch name {
	case "fn", "cmp", "callback", "callable":
		return true
	}
	return false
}

// isRefSetter matches func(any): the wrapper the runner passes for a PHP
// by-reference argument.
func isRefSetter(ft *ast.FuncType) bool {
	if ft.Results != nil && len(ft.Results.List) > 0 {
		return false
	}
	if ft.Params == nil || len(ft.Params.List) != 1 {
		return false
	}
	field := ft.Params.List[0]
	if len(field.Names) > 1 {
		return false
	}
	return phpTypeAST(field.Type) == "mixed"
}

// isContextContext matches the context.Context selector.
func isContextContext(t ast.Expr) bool {
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "context" && sel.Sel.Name == "Context"
}

// phpTypeAST maps a Go type expression to the PHP type name a script sees.
func phpTypeAST(t ast.Expr) string {
	switch v := t.(type) {
	case *ast.Ident:
		return phpTypeName(v.Name)
	case *ast.StarExpr:
		return phpTypeAST(v.X)
	case *ast.SelectorExpr:
		if pkg, ok := v.X.(*ast.Ident); ok && pkg.Name == "model" {
			switch v.Sel.Name {
			case "Array":
				return "array"
			}
		}
		if pkg, ok := v.X.(*ast.Ident); ok && (pkg.Name == "io" || pkg.Name == "fs" || pkg.Name == "os") {
			return "resource"
		}
		return "mixed"
	case *ast.ArrayType:
		if elt, ok := v.Elt.(*ast.Ident); ok && elt.Name == "byte" {
			return "string"
		}
		return "array"
	case *ast.MapType:
		return "array"
	case *ast.InterfaceType:
		return "mixed"
	case *ast.FuncType:
		return "callable"
	case *ast.Ellipsis:
		return phpTypeAST(v.Elt)
	}
	return "mixed"
}

// phpTypeName maps a Go basic type name to PHP's.
func phpTypeName(name string) string {
	switch name {
	case "string":
		return "string"
	case "bool":
		return "bool"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"byte", "rune", "uintptr":
		return "int"
	case "float32", "float64":
		return "float"
	case "error":
		return "error"
	}
	return "mixed"
}

// areaName derives a subsection title from the enclosing function name:
// registerStrings contributes to "strings", a plain Register to none.
func areaName(fn string) string {
	rest := strings.TrimPrefix(fn, "register")
	if rest == fn || rest == "" {
		return ""
	}
	return strings.ReplaceAll(camelToSnake(rest), "_", " ")
}

// recvTypeName returns the receiver's type name, without a pointer star.
func recvTypeName(t ast.Expr) string {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// funcResultTypeName returns the type name of a declaration's first result,
// for a constructor whose class doc lives on the type.
func funcResultTypeName(decl *ast.FuncDecl) string {
	if decl.Type.Results == nil || len(decl.Type.Results.List) == 0 {
		return ""
	}
	t := decl.Type.Results.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
