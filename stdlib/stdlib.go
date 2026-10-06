// Package stdlib provides the forwarded "bring your own standard library" shims
// (the README's register_function mechanism). PHP's stdlib is not reimplemented
// in the VM; instead a curated set of Go functions is registered on a Runtime so
// transpiled PHP can call them by name. This set is sized to run the minitpl
// template engine (the T1 compatibility target).
package stdlib

import (
	"github.com/titpetric/phpscript/runner"
)

// Profile selects which contributed binding areas Mount installs. Every area
// that reaches outside the sandbox is its own bit, so a host's call site names
// each insecure area it enables: Mount(rt, stdlib.Secure|stdlib.Exec).
type Profile = runner.Profile

const (
	// Secure is every binding confined to the runtime's sandbox boundaries.
	Secure = runner.ProfileSecure
	// Exec is process execution, the stdlib/pexec package: exec, system,
	// passthru, shell_exec, the shell quoting pair and the pid pair.
	Exec = runner.ProfileExec

	// Default is the profile for a host running untrusted scripts: the secure
	// surface and nothing else.
	Default = Secure
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

// Mount is Register narrowed to a profile: only the binding areas the bitmask
// names are installed, so a name outside them is undefined in the runtime
// rather than refused, unreachable through a call, a callable or either
// engine. The extra bindings are the caller's own and are installed verbatim.
func Mount(rt *runner.Runtime, profile Profile, bindings ...func(*runner.Runtime)) {
	registerExceptions(rt)

	for _, register := range runner.BindingsFor(profile) {
		register(rt)
	}
	for _, register := range bindings {
		register(rt)
	}

	// How to do this again, for a runtime forked off this one. A binding is a
	// closure over the runtime it was registered on, so a fork has to install
	// its own rather than inherit these - under the same profile.
	rt.SetPreparer(func(child *runner.Runtime) { Mount(child, profile, bindings...) })
}
