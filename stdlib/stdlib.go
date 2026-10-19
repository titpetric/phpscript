// Package stdlib carries the forwarded "bring your own standard library" shims
// (the README's register_function mechanism). PHP's stdlib is not reimplemented
// in the VM; instead Register puts a curated set of Go functions on a Runtime
// so transpiled PHP can call them by name. The set covers the minitpl template
// engine (the T1 compatibility target).
package stdlib

import (
	"github.com/titpetric/phpscript/runner"
)

// Register installs the shims, PHP constants, every binding contributed by an
// imported binding package (see runner.RegisterBinding and imports.go), and
// any bindings the caller passes. The filesystem shims come in that way too,
// rooted at the process working directory; use RegisterFS to bind them to
// another root.
//
// It mounts the process profile, every area unless --stdlib narrowed it, so
// one flag reaches every runtime the process builds.
func Register(rt *runner.Runtime, bindings ...func(*runner.Runtime)) {
	Mount(rt, profile, bindings...)
}
