package formatter

import (
	"strings"

	"github.com/titpetric/phpscript/model"
)

// A go import resolves `http.NewRequest` to the area that binds the package
// while the file parses, and the name the printer holds is the resolved one.
// This file reads the file's own import statements back into the name they
// bound, and the call prints as the source wrote it. A formatter that wrote
// the area name instead would rewrite every member of every imported package
// and leave the import itself naming nothing.

// collectGoImports maps the area of every `import` statement to the name the
// path's last segment bound. It stays nil for a file that writes none.
func collectGoImports(stmts []model.Stmt) map[string]string {
	var out map[string]string
	for _, s := range stmts {
		use, ok := s.(*model.Use)
		if !ok {
			continue
		}
		for _, imp := range use.Imports {
			if imp.Package == "" {
				continue
			}
			if out == nil {
				out = map[string]string{}
			}
			out[imp.Path] = imp.Alias
		}
	}
	return out
}

// goName returns the spelling an imported package's member prints as, and
// reports whether an import of this file holds it. The member is one segment:
// go nests packages and not names.
func (p *printer) goName(name string) (string, bool) {
	if len(p.goImports) == 0 {
		return "", false
	}
	i := strings.LastIndex(name, `\`)
	if i < 0 {
		return "", false
	}
	alias, ok := p.goImports[name[:i]]
	if !ok {
		return "", false
	}
	return alias + "." + name[i+1:], true
}
