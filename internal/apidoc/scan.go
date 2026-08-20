package apidoc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sourceEntry is what the scan knows about one registration site: where it
// is, the doc comment beside it, and the signature when the second argument
// resolved to a function literal or declaration. A nil params or results
// slice means the scan could not see the signature and reflection decides.
type sourceEntry struct {
	pkg     string
	area    string
	comment []string
	params  []Param
	results []string
}

// declInfo is one function declaration the scan indexed for resolution.
type declInfo struct {
	pkg      string
	recvType string // empty for a plain function
	decl     *ast.FuncDecl
}

// typeInfo is a type declaration's doc comment and package.
type typeInfo struct {
	pkg     string
	comment []string
}

// sources is the result of scanning a source tree.
type sources struct {
	funcs map[string]*sourceEntry // by registered PHP name
	ctors map[string]*sourceEntry // by registered class name
	decls map[string][]*declInfo  // by Go function or method name
	types map[string][]typeInfo   // by type name
}

// typeDoc returns the doc comment for a type name. Two packages declaring
// the same type name resolve to stdlib's, the package that registers the
// classes a name collision has occurred over.
func (src *sources) typeDoc(name string) *typeInfo {
	infos := src.types[name]
	if len(infos) == 0 {
		return nil
	}
	for i := range infos {
		if infos[i].pkg == "stdlib" {
			return &infos[i]
		}
	}
	return &infos[0]
}

// skipDirs are directories a scan does not descend into: nothing in them
// registers a binding on the CLI runtime, and demos and tests register
// symbols of their own that would shadow the real sites.
var skipDirs = map[string]bool{
	".git": true, "bin": true, "demos": true, "docs": true,
	"scripts": true, "testdata": true, "tests": true, "vendor": true,
}

// scan parses every non-test Go file under root and collects registration
// sites, function declarations and type doc comments.
func scan(root string) (*sources, error) {
	src := &sources{
		funcs: map[string]*sourceEntry{},
		ctors: map[string]*sourceEntry{},
		decls: map[string][]*declInfo{},
		types: map[string][]typeInfo{},
	}

	fset := token.NewFileSet()
	type parsedFile struct {
		pkg  string
		file *ast.File
	}
	var files []parsedFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		files = append(files, parsedFile{pkg: pkg, file: file})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// First pass: index declarations, so a registration in one file can
	// resolve a function defined in another.
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				info := &declInfo{pkg: pf.pkg, decl: d}
				if d.Recv != nil && len(d.Recv.List) > 0 {
					info.recvType = recvTypeName(d.Recv.List[0].Type)
				}
				src.decls[d.Name.Name] = append(src.decls[d.Name.Name], info)
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					doc := ts.Doc
					if doc == nil {
						doc = d.Doc
					}
					if doc != nil {
						src.types[ts.Name.Name] = append(src.types[ts.Name.Name],
							typeInfo{pkg: pf.pkg, comment: cleanComment(doc)})
					}
				}
			}
		}
	}
	for _, infos := range src.decls {
		sort.SliceStable(infos, func(i, j int) bool { return infos[i].pkg < infos[j].pkg })
	}

	// Second pass: find the registration calls.
	for _, pf := range files {
		cmap := ast.NewCommentMap(fset, pf.file, pf.file.Comments)
		for _, decl := range pf.file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			src.collect(pf.pkg, areaName(fd.Name.Name), fd.Body, cmap)
		}
	}
	return src, nil
}

// collect walks one function body for RegisterFunc and RegisterConstructor
// calls with a literal name.
func (src *sources) collect(pkg, area string, body *ast.BlockStmt, cmap ast.CommentMap) {
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		stack = append(stack, n)

		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		method := sel.Sel.Name
		if method != "RegisterFunc" && method != "RegisterConstructor" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}

		entry := &sourceEntry{pkg: pkg, area: area}
		entry.comment = stmtComment(cmap, stack)
		src.resolveSignature(entry, pkg, name, call.Args[1])

		table := src.funcs
		if method == "RegisterConstructor" {
			table = src.ctors
		}
		if prev, ok := table[name]; !ok || (len(prev.comment) == 0 && len(entry.comment) > 0) {
			table[name] = entry
		}
		return true
	})
}

// resolveSignature fills entry from the registered expression: a function
// literal carries its signature inline, an identifier or selector resolves
// through the declaration index. The doc comment of a resolved declaration
// backs an absent registration-site comment, with its leading Go name
// rewritten to the PHP name so the published line reads as PHP documentation.
func (src *sources) resolveSignature(entry *sourceEntry, pkg, name string, expr ast.Expr) {
	var ft *ast.FuncType
	var doc []string
	switch fn := expr.(type) {
	case *ast.FuncLit:
		ft = fn.Type
	case *ast.Ident:
		if info := src.lookupDecl(pkg, fn.Name, true); info != nil {
			ft = info.decl.Type
			doc = declComment(info.decl, name)
			if len(entry.comment) == 0 && len(doc) == 0 {
				if resultType := funcResultTypeName(info.decl); resultType != "" {
					if ti := src.typeDoc(resultType); ti != nil {
						doc = ti.comment
					}
				}
			}
		}
	case *ast.SelectorExpr:
		if info := src.lookupDecl(pkg, fn.Sel.Name, false); info != nil {
			ft = info.decl.Type
			doc = declComment(info.decl, name)
		}
	}
	if ft != nil {
		entry.params, entry.results = signature(ft)
	}
	if len(entry.comment) == 0 {
		entry.comment = doc
	}
}

// method returns the indexed declaration for a method on typeName, as an
// entry carrying its comment and signature, or nil.
func (src *sources) method(typeName, goName string) *sourceEntry {
	for _, info := range src.decls[goName] {
		if info.recvType != typeName {
			continue
		}
		entry := &sourceEntry{comment: declComment(info.decl, camelToSnake(goName))}
		entry.params, entry.results = signature(info.decl.Type)
		return entry
	}
	return nil
}

// lookupDecl finds a declaration by name, preferring the calling package.
// samePkg restricts the match to it, which is how a bare identifier binds.
func (src *sources) lookupDecl(pkg, name string, samePkg bool) *declInfo {
	var fallback *declInfo
	for _, info := range src.decls[name] {
		if info.pkg == pkg {
			return info
		}
		if fallback == nil {
			fallback = info
		}
	}
	if samePkg {
		return nil
	}
	return fallback
}

// stmtComment returns the comment written against the statement enclosing
// the current inspect position: the leading group when there is one, else a
// trailing comment on the same line.
func stmtComment(cmap ast.CommentMap, stack []ast.Node) []string {
	for i := len(stack) - 1; i >= 0; i-- {
		stmt, ok := stack[i].(ast.Stmt)
		if !ok {
			continue
		}
		groups := cmap[stmt]
		if len(groups) == 0 {
			return nil
		}
		for _, g := range groups {
			if g.End() < stmt.Pos() {
				return cleanComment(g)
			}
		}
		return cleanComment(groups[0])
	}
	return nil
}

// declComment returns a declaration's doc comment with the godoc-style
// leading symbol name replaced by the registered PHP name.
func declComment(decl *ast.FuncDecl, phpName string) []string {
	if decl.Doc == nil {
		return nil
	}
	lines := cleanComment(decl.Doc)
	if len(lines) > 0 {
		first, rest, ok := strings.Cut(lines[0], " ")
		if ok && first == decl.Name.Name {
			lines[0] = phpName + " " + rest
		}
	}
	return lines
}

// cleanComment strips comment markers and directives, returning plain lines.
func cleanComment(group *ast.CommentGroup) []string {
	var lines []string
	for _, c := range group.List {
		text := c.Text
		switch {
		case strings.HasPrefix(text, "//"):
			line := strings.TrimPrefix(strings.TrimPrefix(text, "//"), " ")
			if strings.HasPrefix(line, "go:") || strings.HasPrefix(line, "nolint") {
				continue
			}
			lines = append(lines, line)
		case strings.HasPrefix(text, "/*"):
			text = strings.TrimSuffix(strings.TrimPrefix(text, "/*"), "*/")
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
				lines = append(lines, line)
			}
		}
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

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

// paramName converts a Go parameter name to the PHP spelling, giving the
// blank identifier a name that says the value is accepted and ignored.
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
