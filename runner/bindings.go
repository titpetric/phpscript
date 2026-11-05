package runner

import (
	"sync"
)

// binding pairs an installer with the profile area it belongs to and the
// namespace area that names it.
type binding struct {
	name    string
	layer   int
	profile Profile
	install func(*Runtime)
}

// bindings holds the runtime installers contributed by binding packages.
var bindings struct {
	sync.Mutex
	installers []binding
}

// RegisterBinding contributes a runtime installer under ProfileSecure, in the
// area NamespaceUnnamed. A binding package names its area with RegisterArea.
func RegisterBinding(installer func(*Runtime)) {
	RegisterProfileArea(NamespaceUnnamed, ProfileSecure, installer)
}

// RegisterProfileBinding contributes a runtime installer under the given area,
// for a binding that leaves the sandbox: stdlib/ps/pexec registers under
// ProfileExec. stdlib.Mount installs it only for a host whose profile names
// the area; a zero profile is never installed.
func RegisterProfileBinding(profile Profile, installer func(*Runtime)) {
	RegisterProfileArea(NamespaceUnnamed, profile, installer)
}

// Bindings returns every contributed installer, whatever its area.
func Bindings() []func(*Runtime) {
	return BindingsFor(^Profile(0))
}

// BindingsFor returns the contributed installers whose area overlaps profile,
// in install order: the Runtime\PS areas first and the Runtime\PX areas over
// them, each in registration order.
func BindingsFor(profile Profile) []func(*Runtime) {
	areas := AreasFor(profile)
	installers := make([]func(*Runtime), len(areas))
	for i, area := range areas {
		installers[i] = area.Install
	}
	return installers
}
