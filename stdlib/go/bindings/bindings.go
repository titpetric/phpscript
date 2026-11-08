// Package bindings installs a go package as the area a script imports it by.
// A binding package names what it binds, one entry per name, and this package
// resolves the area, registers the names on the runtime that imported them and
// enumerates the types the signatures reach.
//
// A name is a triple: the import path, the name go spells it, and the value.
//
//	bindings.For("net/http").
//		Bind("NewRequest", http.NewRequest).
//		Bind("StatusOK", http.StatusOK).
//		Install()
//
// `import "net/http"` then installs the area and binds `http.NewRequest` and
// `http.StatusOK` for the file that wrote it. See
// docs/reference/extensions/go-imports.md.
package bindings

import (
	"fmt"

	"github.com/titpetric/phpscript/modelutil"
)

// Entry is one name a package binds: the import path, the name go spells, the
// value behind it. A func binds as a function of the area; every other value
// binds as one of its constants.
type Entry struct {
	Package string
	Name    string
	Value   any
}

// For opens the set of a go import path. It panics on a path outside the
// standard library subset this build already links, which is a defect in the
// binding package and no condition a program meets.
func For(path string) *Set {
	area, _, ok := modelutil.GoImportArea(path)
	if !ok {
		panic(fmt.Sprintf("bindings: %q is no go package path", path))
	}
	if !Linked(path) {
		panic(fmt.Sprintf("bindings: %q is outside the linked standard library", path))
	}
	return &Set{path: path, area: area}
}
