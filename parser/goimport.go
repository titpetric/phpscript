package parser

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// `import "net/http"` is the go runtime's second import, and the overlay the
// runtime lays over this grammar. A file that opened with `use Runtime\Go;`
// may write it; every other file reads `import` as a name of its own, and the
// statement, the dot and the packages stay out of php's grammar.
//
// What it binds is an area: the path resolves through model.GoImportArea, the
// statement is the `use Runtime\Go\Net\HTTP` the runtime already installs, and
// the name is the path's last segment. `http.NewRequest(...)` resolves to
// `Runtime\Go\Net\HTTP\NewRequest` while the file parses, the one place a `.`
// names something and does not concatenate. The file said which names a
// package holds, and the parser reads the pair as one name.
//
// The name binds per file, as every import does. A second file of the same
// program writes its own `import` line or reaches the area by its name.

// atGoImport reports whether the statement ahead is a go import: the keyword,
// a quoted path, and a file that bound the go runtime to read them.
func (p *parser) atGoImport() bool {
	return p.goRuntime && p.peek(1).kind == tString
}

// parseGoImport reads `import "net/http";` into the `use` of the area that
// binds the package. It refuses a path the standard library does not spell.
func (p *parser) parseGoImport() (model.Stmt, error) {
	line := p.next().line // import
	path := p.next().val
	area, alias, ok := modelutil.GoImportArea(path)
	if !ok {
		return nil, fmt.Errorf("line %d: import: %q is no go package path", line, path)
	}
	if p.goPkgs == nil {
		p.goPkgs = map[string]string{}
	}
	p.goPkgs[alias] = area
	p.optSemi()
	return &model.Use{Imports: []model.UseImport{{Path: area, Alias: alias, Package: path}}}, nil
}

// goPackage returns the area an imported package name stands for. The name is
// go's, and the comparison is exact where php folds a function's case: a file
// that wrote `HTTP.NewRequest` meant no package this import bound.
func (p *parser) goPackage(name string) (string, bool) {
	area, ok := p.goPkgs[name]
	return area, ok
}

// atGoMember reports whether the member spelling, `.Name`, stands where the
// parser is.
func (p *parser) atGoMember() bool {
	return p.isOp(".") && p.peek(1).kind == tIdent
}

// goMember folds `http.Request` into the area name the import resolved it to,
// and answers the source spelling beside it for whoever prints the file back.
// A name no import bound comes back unchanged.
func (p *parser) goMember(name string) (full, source string, ok bool) {
	area, bound := p.goPackage(name)
	if !bound || !p.atGoMember() {
		return name, name, false
	}
	p.next() // .
	member := p.next().val
	return area + `\` + member, name + "." + member, true
}
