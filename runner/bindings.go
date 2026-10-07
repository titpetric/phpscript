package runner

import (
	"sync"
)

// Profile is a bitmask naming the binding areas a host mounts. Each area that
// reaches outside the runtime's sandbox is its own bit, so a call site spells
// out every insecure area it enables; stdlib.Mount reads one.
type Profile uint64

const (
	// ProfileSecure is every binding confined to the runtime's own sandbox
	// boundaries: the surface for a host running untrusted scripts.
	ProfileSecure Profile = 1 << iota

	// ProfileExec is process execution, contributed by stdlib/pexec. A command
	// runs with the permissions of the user running the host, outside every
	// filesystem boundary.
	ProfileExec
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
// stdlib.Register runs on every Runtime it sets up. Binding packages call it
// from their init() (see stdlib/core/defer.go), so a package is wired in by
// importing it, the way a program imports a database/sql driver. The registry
// lives here, in the leaf package that owns Runtime, so that stdlib can
// blank-import its subpackages without the two importing each other.
//
// A binding that reaches outside the runtime's sandbox registers through
// RegisterProfileBinding with its area bit instead.
//
// Installers run in registration order, before the bindings a host passes to
// stdlib.Register directly.
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

// SetProfile records the area mask this runtime's bindings were installed
// under. stdlib.Mount calls it, so that a later reroot of one area can ask
// whether the mount included it.
func (rt *Runtime) SetProfile(profile Profile) { rt.profile = profile }

// Profile reports the area mask this runtime's bindings were installed under.
func (rt *Runtime) Profile() Profile { return rt.profile }

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
