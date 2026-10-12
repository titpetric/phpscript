package stdlib

import (
	"github.com/titpetric/phpscript/runner"
)

// Mount is Register narrowed to a profile: only the binding areas the bitmask
// names are installed, so a name outside them is undefined in the runtime
// and never refused, unreachable through a call, a callable or either
// engine. The extra bindings are the caller's own and are installed verbatim.
func Mount(rt *runner.Runtime, profile Profile, bindings ...func(*runner.Runtime)) {
	rt.SetProfile(profile)
	registerExceptions(rt)

	for _, register := range runner.BindingsFor(profile) {
		register(rt)
	}
	for _, register := range bindings {
		register(rt)
	}

	// How to do this again, for a runtime forked off this one. A binding is a
	// closure over the runtime it was registered on, so a fork has to install
	// its own and inherits none of these, under the same profile.
	rt.SetPreparer(func(child *runner.Runtime) { Mount(child, profile, bindings...) })
}
