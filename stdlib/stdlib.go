// Package stdlib holds the forwarded "bring your standard library" shims
// (the README's register_function mechanism). PHP's stdlib is not reimplemented
// in the VM; instead Register puts a curated set of Go functions on a Runtime
// so transpiled PHP can call each by name. The set covers the minitpl template
// engine (the T1 compatibility target).
package stdlib

import (
	"github.com/titpetric/phpscript/runner"
)

// Register installs the shims, plus every binding, using the process profile.
func Register(rt *runner.Runtime, bindings ...func(*runner.Runtime)) {
	Mount(rt, profile, bindings...)
}
