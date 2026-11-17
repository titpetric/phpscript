package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/compat"
	pshttp "github.com/titpetric/phpscript/stdlib/ps/http"
)

// Mount is Register narrowed to a profile: it installs only the binding areas
// the bitmask names and a name from any other area stays undefined in the
// runtime and never refused, unreachable through a call, a callable or either
// engine. Mount installs the caller's extra bindings verbatim.
func Mount(rt *runner.Runtime, profile Profile, bindings ...func(*runner.Runtime)) {
	rt.SetProfile(profile)
	if rt.GoRuntime() {
		// The go runtime keeps echo's output buffers and the request functions; its errors are go's and it names no throwable class.
		compat.RegisterBuffers(rt)
		pshttp.RegisterRequest(rt)
	} else {
		registerErrors(rt)
		registerExceptions(rt)
	}

	for _, area := range runner.SelectAreas(runner.AreasFor(profile), rt.Areas()) {
		area.Install(rt)
	}
	for _, register := range bindings {
		register(rt)
	}

	// How to do this again, for a runtime forked off this one. A binding is a
	// closure over its registering runtime and a fork has to install
	// its own and inherits none of these, under the same profile.
	rt.SetPreparer(func(child *runner.Runtime) { Mount(child, profile, bindings...) })
}
