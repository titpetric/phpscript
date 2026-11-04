package parser

import (
	"strings"

	"github.com/titpetric/phpscript/model"
)

// The go runtime is the one binding area a file opts into. `use Runtime\Go`
// binds go's builtin names for the rest of that file: a bare `make(...)` is the
// binding and not a name php resolves, and the call prints as the source wrote
// it, so the formatter round-trips.
//
// The binding is per file, as every `use` is, and it wins over the enclosing
// namespace: a file that imported go's `make` meant go's, which is how php
// reads an imported name against one it would otherwise resolve. It binds the
// spelling go uses and no other, where php folds a function's case: a file that
// writes `Make()` asked php for it.
//
// A qualified call decides for itself and needs no import, here and in the
// runtime both: `\strlen($s)` is php's whatever the file imported, and
// `Runtime\Go\make(...)` is go's.

// useGo records the import of the go runtime, the one import the parser reads
// for itself. A leading separator and the namespace's case are php's to ignore.
func (p *parser) useGo(name string) {
	if strings.EqualFold(strings.Trim(name, "\\"), model.GoNamespace) {
		p.goRuntime = true
	}
}

// goBound reports whether a bare name is a go builtin this file's imports bind.
func (p *parser) goBound(name string) bool {
	return p.goRuntime && model.GoBuiltin(name)
}
