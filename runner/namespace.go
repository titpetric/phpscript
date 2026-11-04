package runner

import (
	"strings"
	"sync"

	"github.com/titpetric/phpscript/model"
)

// A namespace binding is a binding area a script imports by name. The
// installers in bindings.go are the surface every runtime carries; these are
// the ones a program opts into with `use Namespace`, and a runtime that saw no
// such import answers for none of their names. An opt-in area stays out of
// function_exists, get_defined_functions and the generated reference, where
// stdlib.Mount's areas all appear.
//
// A namespace registers and resolves under a case-insensitive key, as every
// other name PHP resolves does, and the import drops a leading separator:
// `use \Runtime\Go` and `use Runtime\Go` name the same namespace.
//
// The install outlives the program that imported it, and no call un-registers
// a binding: a runtime serving a second program carries what the first one
// imported. The parser decides the names a script may call, per file, and the
// import binds them there.

// namespaces holds the installers contributed per namespace.
var namespaces struct {
	sync.Mutex
	installers map[string]func(*Runtime)
}

// RegisterNamespace contributes the installer a `use name` import runs. It
// drops a nil installer and an empty name.
func RegisterNamespace(name string, installer func(*Runtime)) {
	if name == "" || installer == nil {
		return
	}
	namespaces.Lock()
	defer namespaces.Unlock()
	if namespaces.installers == nil {
		namespaces.installers = map[string]func(*Runtime){}
	}
	namespaces.installers[foldName(name)] = installer
}

// NamespaceInstaller returns the installer contributed for name.
func NamespaceInstaller(name string) (func(*Runtime), bool) {
	namespaces.Lock()
	defer namespaces.Unlock()
	installer, ok := namespaces.installers[foldName(name)]
	return installer, ok
}

// UseNamespace installs the bindings contributed for an imported namespace,
// once per runtime, and reports whether it installed any.
func (rt *Runtime) UseNamespace(name string) bool {
	installer, ok := NamespaceInstaller(name)
	if !ok {
		return false
	}
	key := foldName(name)
	if _, done := rt.imported[key]; done {
		return false
	}
	if rt.imported == nil {
		rt.imported = map[string]struct{}{}
	}
	rt.imported[key] = struct{}{}
	installer(rt)
	return true
}

// useImport installs what one import names: the namespace, or the namespace a
// `use function` or `use const` import names a symbol in.
func (rt *Runtime) useImport(path string) {
	if rt.UseNamespace(path) {
		return
	}
	if i := strings.LastIndexByte(path, '\\'); i > 0 {
		rt.UseNamespace(path[:i])
	}
}

// useImports installs the namespace bindings the statements import.
//
// Both engines pass through here. The bytecode compiler resolves a call
// through the runtime's function table at run time, and an install before
// execution serves it as it serves the interpreter.
func (rt *Runtime) useImports(stmts []model.Stmt) {
	for _, s := range stmts {
		use, ok := s.(*model.Use)
		if !ok {
			continue
		}
		for _, imported := range use.Imports {
			rt.useImport(imported.Path)
		}
	}
}

// foldName is the key a namespace registers and resolves under.
func foldName(name string) string {
	return strings.ToLower(strings.Trim(name, "\\"))
}
