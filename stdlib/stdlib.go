// Package stdlib provides the forwarded "bring your own standard library" shims
// (the README's register_function mechanism). PHP's stdlib is not reimplemented
// in the VM; instead a curated set of Go functions is registered on a Runtime so
// transpiled PHP can call them by name. This set is sized to run the minitpl
// template engine (the T1 compatibility target).
package stdlib

import (
	"github.com/titpetric/phpscript/runner"
)

// Register installs the shims, PHP constants, every
// binding contributed by an imported binding package (see
// runner.RegisterBinding and imports.go), and any additional bindings passed by
// the caller. The filesystem shims come in that way too, rooted at the process
// working directory; use RegisterFS to bind them to another root. It is the
// whole surface, every profile area included, which is what a CLI run, the
// fixture runner and the demos expect; a host serving untrusted scripts calls
// Mount with the profile it grants instead.
func Register(rt *runner.Runtime, bindings ...func(*runner.Runtime)) {
	Mount(rt, ^Profile(0), bindings...)
}
