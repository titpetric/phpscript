package lint

import (
	"io"
	"sync"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// An opt-in binding area installs on the statement that names it, `use
// Runtime\Go\Strings;` or `import "strings";`, and the name check runs no
// statement: the registry in names.go holds the default surface alone, where
// every call and class under an area reads as undefined. The check reads the
// area's own runtime instead, one per area and per process, and a file that
// imports nothing reads none of them.

var areaRuntimes = struct {
	sync.Mutex
	byArea map[string]*runner.Runtime
}{byArea: map[string]*runner.Runtime{}}

// programAreas lists the binding areas the use and import statements of prog
// install, wherever the file writes them. A path no installer answers for is a
// php namespace and belongs to the registry.
func programAreas(prog *model.Program) []string {
	var areas []string
	seen := map[string]bool{}
	w := &astWalker{prog: prog}
	w.stmt = func(s model.Stmt) {
		use, ok := s.(*model.Use)
		if !ok || use.Kind != "" {
			return
		}
		for _, name := range use.Imports {
			if seen[name.Path] {
				continue
			}
			if _, ok := runner.NamespaceInstaller(name.Path); ok {
				seen[name.Path] = true
				areas = append(areas, name.Path)
			}
		}
	}
	w.walk(prog.Stmts)
	return areas
}

// areaRuntime answers the runtime an area installed over the default surface.
func areaRuntime(area string) *runner.Runtime {
	areaRuntimes.Lock()
	defer areaRuntimes.Unlock()
	if rt, ok := areaRuntimes.byArea[area]; ok {
		return rt
	}
	rt := runner.New(io.Discard, runner.Options{SAPI: "cli"})
	stdlib.Register(rt)
	rt.UseNamespace(area)
	areaRuntimes.byArea[area] = rt
	return rt
}

// areaFunc reports whether one of the areas binds the function name.
func areaFunc(areas []string, name string) bool {
	for _, area := range areas {
		if areaRuntime(area).FunctionExists(name) {
			return true
		}
	}
	return false
}

// areaClass reports whether one of the areas installs the class.
func areaClass(areas []string, name string) bool {
	for _, area := range areas {
		if known, _ := areaRuntime(area).ClassExists(name, false); known {
			return true
		}
	}
	return false
}
