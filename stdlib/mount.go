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
