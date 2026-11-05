package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// finding is one binding the rules refuse: the position, the name a script
// calls it by, and what to do about it.
type finding struct {
	file    string
	line    int
	binding string
	message string
}

// String renders the finding the way a compiler reports one, and an editor
// jumps to the position from it.
func (f finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.file, f.line, f.binding, f.message)
}

// pkg is one directory's Go source, test files excluded: a binding is what a
// shipped build registers and a test file registers nothing.
type pkg struct {
	dir   string
	root  string
	fset  *token.FileSet
	files []*ast.File

	// funcs holds every declaration under the bare name a call site writes,
	// methods included: two types of one package declare a Write apiece, and
	// one key answering with the last indexed would read a narrowing in the
	// wrong body. Every declaration of the name answers, which over-reports a
	// collision and hides none.
	funcs   map[string][]*ast.FuncDecl
	methods map[string][]*ast.FuncDecl
}

// lint reports every binding in the tree at root that points at a runtime
// value type. It reads one directory at a time: a binding and the helpers it
// passes a script value to sit in one package, and the rules need no more
// context than that.
func lint(root string) ([]finding, error) {
	var out []finding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (name == "testdata" || name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}
		p, err := parseDir(root, path)
		if err != nil {
			return err
		}
		if p != nil {
			out = append(out, p.check()...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].line < out[j].line
	})
	return out, nil
}

// parseDir parses the Go files directly in dir, and returns nil for a
// directory that holds none.
func parseDir(root, dir string) (*pkg, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	p := &pkg{
		dir:     dir,
		root:    root,
		fset:    token.NewFileSet(),
		funcs:   map[string][]*ast.FuncDecl{},
		methods: map[string][]*ast.FuncDecl{},
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(p.fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			// A file the parser refuses is a build failure, and the
			// compiler reports it with more than this linter knows. It
			// contributes no binding and no second opinion.
			continue
		}
		p.files = append(p.files, file)
	}
	if len(p.files) == 0 {
		return nil, nil
	}
	p.index()
	return p, nil
}

// index records every function the package declares, by name and by the type
// holding it as a method.
func (p *pkg) index() {
	for _, file := range p.files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			p.funcs[fn.Name.Name] = append(p.funcs[fn.Name.Name], fn)
			if owner := receiverType(fn); owner != "" {
				p.methods[owner] = append(p.methods[owner], fn)
			}
		}
	}
}

// receiverType is the name of the type a method hangs off, pointer receiver
// included, and "" for a plain function.
func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	ident, ok := t.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// binding is one Go function a script reaches: a registered function, a
// constructor, or a method of the type a constructor returns.
type binding struct {
	name  string
	ftype *ast.FuncType
	body  *ast.BlockStmt
	pos   token.Pos
}

// check applies both rules to every binding the package registers, reporting
// each site once: one helper serves four bindings in stdlib/ps/core alone, and
// the fix is one edit.
func (p *pkg) check() []finding {
	var out []finding
	seen := map[string]bool{}
	for _, b := range p.bindings() {
		for _, f := range append(p.checkSignature(b), p.checkNarrowing(b)...) {
			key := f.file + ":" + strconv.Itoa(f.line)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

// position renders a node's position with the file path relative to the lint
// root, the spelling a reader pastes back into an editor.
func (p *pkg) position(pos token.Pos) (string, int) {
	at := p.fset.Position(pos)
	file := at.Filename
	if rel, err := filepath.Rel(p.root, file); err == nil {
		file = rel
	}
	return filepath.ToSlash(file), at.Line
}

// bindings collects what the package registers: the function value behind
// every RegisterFunc, the constructor behind every RegisterConstructor, and
// the exported methods of the type that constructor returns, which are the
// methods a script calls on the class.
func (p *pkg) bindings() []binding {
	var out []binding
	for _, file := range p.files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "RegisterFunc":
				out = append(out, p.registered(call, "")...)
			case "RegisterConstructor":
				out = append(out, p.registered(call, registeredName(call))...)
			}
			return true
		})
	}
	return out
}

// registered resolves the function value a registration names: the literal
// written at the call, or the function it names in this package. A value from
// another package is out of reach of a directory at a time, and a computed one
// (`rt.RegisterFunc(name, fn)` over a table) names no function to read.
//
// With class set, the methods of the type the constructor returns join the
// list: a script calls them the same way it calls a function.
func (p *pkg) registered(call *ast.CallExpr, class string) []binding {
	name := registeredName(call)
	var out []binding
	for _, fn := range p.resolve(call.Args[1]) {
		out = append(out, binding{name: name, ftype: fn.ftype, body: fn.body, pos: fn.pos})
		if class == "" {
			continue
		}
		for _, method := range p.methods[constructedType(fn.ftype)] {
			if !method.Name.IsExported() {
				continue
			}
			out = append(out, binding{
				name:  class + "::" + method.Name.Name,
				ftype: method.Type,
				body:  method.Body,
				pos:   method.Pos(),
			})
		}
	}
	return out
}

// function is one resolved function value: its signature, its body and where
// a finding points.
type function struct {
	ftype *ast.FuncType
	body  *ast.BlockStmt
	pos   token.Pos
}

// resolve reads the function value a registration names, in the four
// spellings this tree registers in: the literal written at the call, a
// function or method of this package by name, a method value
// (`rt.RegisterFunc("exec", r.phpExec)`), and the literal a factory returns
// (`rt.RegisterFunc("trim", phpTrim(" "))`).
//
// A value from another package is out of reach of a directory at a time, and a
// computed one (`rt.RegisterFunc(name, fn)` over a table) names no function to
// read.
func (p *pkg) resolve(arg ast.Expr) []function {
	switch fn := arg.(type) {
	case *ast.FuncLit:
		return []function{{ftype: fn.Type, body: fn.Body, pos: fn.Pos()}}
	case *ast.Ident:
		return p.declared(fn.Name)
	case *ast.SelectorExpr:
		// A method value, or a function of another package: the index answers
		// for the first and holds nothing for the second.
		return p.declared(fn.Sel.Name)
	case *ast.CallExpr:
		var out []function
		for _, factory := range p.declared(calledName(fn)) {
			if lit := returnedLiteral(factory.body); lit != nil {
				out = append(out, function{ftype: lit.Type, body: lit.Body, pos: lit.Pos()})
			}
		}
		return out
	}
	return nil
}

// declared returns every function of the package under one name.
func (p *pkg) declared(name string) []function {
	var out []function
	for _, decl := range p.funcs[name] {
		if decl.Body == nil {
			continue
		}
		out = append(out, function{ftype: decl.Type, body: decl.Body, pos: decl.Pos()})
	}
	return out
}

// returnedLiteral is the closure a factory returns, which is the binding a
// registration of its call registers.
func returnedLiteral(body *ast.BlockStmt) *ast.FuncLit {
	if body == nil {
		return nil
	}
	var lit *ast.FuncLit
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || lit != nil || len(ret.Results) == 0 {
			return true
		}
		if found, ok := ret.Results[0].(*ast.FuncLit); ok {
			lit = found
		}
		return true
	})
	return lit
}

// registeredName is the name a script types for a registration, and "" when
// the call computes it.
func registeredName(call *ast.CallExpr) string {
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	name, err := strconv.Unquote(lit.Value)
	if err != nil {
		return phpName(strings.Trim(lit.Value, `"`))
	}
	return name
}

// constructedType names the type a constructor returns, the type carrying the
// methods a script calls on the class. A constructor returning a type from
// another package, or an interface, names no method set here: the methods hang
// off the implementation, under a name only the type checker resolves.
func constructedType(ftype *ast.FuncType) string {
	if ftype.Results == nil || len(ftype.Results.List) == 0 {
		return ""
	}
	t := ftype.Results.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	ident, ok := t.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// label names a binding in a finding: the name a script types, or the position
// alone for a registration whose name a table holds.
func (b binding) label() string {
	if b.name == "" {
		return "binding"
	}
	return phpName(b.name)
}
