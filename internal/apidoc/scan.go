package apidoc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sourceEntry holds one registration site's package, area, doc comment and signature.
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

// typeInfo is a type declaration's doc comment, package and field comments.
type typeInfo struct {
	pkg     string
	comment []string
	fields  map[string][]string // doc comment by Go field name
}

// sources is the result of scanning a source tree.
type sources struct {
	funcs map[string]*sourceEntry // by registered PHP name
	ctors map[string]*sourceEntry // by registered class name
	decls map[string][]*declInfo  // by Go function or method name
	types map[string][]typeInfo   // by type name
	paths map[string]*typeInfo    // by import path and type name
}

// typeDoc returns the doc comment for name, preferring the stdlib package's declaration.
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

// skipDirs names the directories a scan does not descend into.
var skipDirs = map[string]bool{
	".git": true, "bin": true, "demos": true, "docs": true,
	"scripts": true, "testdata": true, "tests": true, "vendor": true,
}

// emptySources returns a source set whose every lookup misses.
func emptySources() *sources {
	return &sources{
		funcs: map[string]*sourceEntry{},
		ctors: map[string]*sourceEntry{},
		decls: map[string][]*declInfo{},
		types: map[string][]typeInfo{},
		paths: map[string]*typeInfo{},
	}
}

// scan parses every non-test Go file under root for registrations, declarations and type docs.
func scan(root string) (*sources, error) {
	src := emptySources()

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

	// First pass: index declarations and a registration in one file can
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
					info := typeInfo{pkg: pf.pkg, fields: fieldComments(ts.Type)}
					// The by-name table holds a commented type and no
					// other, the set typeDoc picks from. Widening it would
					// let a type holding field comments and no doc comment
					// shadow a namesake in another package that has one.
					if doc != nil {
						info.comment = cleanComment(doc)
						src.types[ts.Name.Name] = append(src.types[ts.Name.Name], info)
					}
					src.paths[importPath(pf.pkg)+"."+ts.Name.Name] = &info
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

// collect walks one function body for RegisterFunc and RegisterConstructor calls with a literal name.
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

// resolveSignature fills a registration's params and results from a literal, identifier or selector.
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
		// The qualifier has to agree with the package holding the declaration
		// found in. Without that check a registration of regexp.Compile reads
		// the signature of flatstack/engine.Compile, which shares the name and
		// that alone, and publishes its parameters.
		if qualifier, ok := fn.X.(*ast.Ident); ok {
			if info := src.lookupDeclIn(pkg, qualifier.Name, fn.Sel.Name); info != nil {
				ft = info.decl.Type
				doc = declComment(info.decl, name)
			}
		}
	}
	if ft != nil {
		entry.params, entry.results = signature(ft)
	}
	if len(entry.comment) == 0 {
		entry.comment = doc
	}
}

// method returns the declaration of goName on typeName as an entry, or nil.
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

// lookupDecl finds a declaration by name, preferring pkg and, with samePkg, excluding any other.
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

// lookupDeclIn resolves a qualified selector to a same-package function or a method on the qualifier.
func (src *sources) lookupDeclIn(pkg, qualifier, name string) *declInfo {
	var method *declInfo
	for _, info := range src.decls[name] {
		switch {
		case info.pkg == pkg, path.Base(info.pkg) == qualifier:
			return info
		case info.recvType != "" && method == nil:
			method = info
		}
	}
	return method
}
