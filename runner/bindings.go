package runner

import (
	"sync"
)

// binding pairs an installer with the profile area it belongs to.
type binding struct {
	profile Profile
	install func(*Runtime)
}

// bindings holds the runtime installers contributed by binding packages.
var bindings struct {
	sync.Mutex
	installers []binding
}

// RegisterBinding contributes a runtime installer under ProfileSecure, which
// stdlib.Register runs on every Runtime it sets up. Installers run in
// registration order, before the bindings a host passes to stdlib.Register
// directly. A binding that reaches outside the sandbox registers through
// RegisterProfileBinding with its area bit instead.
//
// Binding packages call this from their init() (see stdlib/core/defer.go), so
// importing a package wires it in. Why the registry lives in the leaf package
// that owns Runtime is docs/reference/extensions/bindings.md.
func RegisterBinding(installer func(*Runtime)) {
	RegisterProfileBinding(ProfileSecure, installer)
}

// RegisterProfileBinding contributes a runtime installer under the given area,
// for a binding that leaves the sandbox: stdlib/pexec registers under
// ProfileExec. stdlib.Mount installs it only for a host whose profile names
// the area; a zero profile is never installed.
func RegisterProfileBinding(profile Profile, installer func(*Runtime)) {
	if installer == nil {
		return
	}
	bindings.Lock()
	defer bindings.Unlock()
	bindings.installers = append(bindings.installers, binding{profile: profile, install: installer})
}

// Bindings returns every contributed installer in registration order,
// regardless of area; stdlib.Register runs the whole surface.
func Bindings() []func(*Runtime) {
	return BindingsFor(^Profile(0))
}

// BindingsFor returns the contributed installers whose area overlaps profile,
// in registration order.
func BindingsFor(profile Profile) []func(*Runtime) {
	bindings.Lock()
	defer bindings.Unlock()
	installers := make([]func(*Runtime), 0, len(bindings.installers))
	for _, contributed := range bindings.installers {
		if contributed.profile&profile != 0 {
			installers = append(installers, contributed.install)
		}
	}
	return installers
}
