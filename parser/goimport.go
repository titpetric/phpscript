package parser

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// `import "net/http"` is the go runtime's second import, an overlay the runtime
// lays over this grammar. A file that opened with `use Runtime\Go;` may write
// it; any other file reads `import` as an ordinary name, and the statement, the
// dot, the packages stay out of php's grammar.
//
// The binding names an area: the path resolves through modelutil.GoImportArea,
// the statement is the `use Runtime\Go\Net\HTTP` the runtime already installs,
// and the path's last segment is the name. `http.NewRequest(...)` resolves to
// `Runtime\Go\Net\HTTP\NewRequest` while the file parses, the only place a `.`
// names something and does not concatenate. The file said which names a
// package holds; the parser reads the pair as a single name.
//
// The name binds per file, as every import does. The statement belongs at the
// top level: the runtime installs the area before the file runs, from the
// statements there. A nested import is a parse error, refused at the line
// that writes it: it would otherwise resolve a name the runtime never installs.
// A second file of the same program writes its own `import` line or reaches
// the area by its name.

// atGoImport reports whether the statement ahead is a go import.
func (p *parser) atGoImport() bool {
	return p.goRuntime && p.peek(1).kind == tString
}

// parseGoImport reads import "path" into a use of its area, refusing unknown paths.
func (p *parser) parseGoImport() (model.Stmt, error) {
	line := p.next().line // import
	if !p.topLevel {
		return nil, fmt.Errorf("line %d: import is only allowed at the top level", line)
	}
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

// goPackage returns the area name resolves to, matched byte for byte.
func (p *parser) goPackage(name string) (string, bool) {
	area, ok := p.goPkgs[name]
	return area, ok
}

// atGoMember reports whether a ".Name" member spelling starts at the cursor.
func (p *parser) atGoMember() bool {
	return p.isOp(".") && p.peek(1).kind == tIdent
}

// goMember folds name.member into its area path, answering the source spelling.
func (p *parser) goMember(name string) (full, source string, ok bool) {
	area, bound := p.goPackage(name)
	if !bound || !p.atGoMember() {
		return name, name, false
	}
	p.next() // .
	member := p.next().val
	return area + `\` + member, name + "." + member, true
}
