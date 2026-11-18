package runner

import (
	"sync"
)

// binding pairs an installer with the profile and namespace areas that admit it.
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

// RegisterBinding contributes installer under ProfileSecure, in the area NamespaceUnnamed.
func RegisterBinding(installer func(*Runtime)) {
	RegisterProfileArea(NamespaceUnnamed, ProfileSecure, installer)
}

// RegisterProfileBinding contributes installer under profile for stdlib.Mount.
func RegisterProfileBinding(profile Profile, installer func(*Runtime)) {
	RegisterProfileArea(NamespaceUnnamed, profile, installer)
}

// Bindings returns every contributed installer, whatever its area.
func Bindings() []func(*Runtime) {
	return BindingsFor(^Profile(0))
}

// BindingsFor returns the installers whose areas overlap profile, in install order.
func BindingsFor(profile Profile) []func(*Runtime) {
	areas := AreasFor(profile)
	installers := make([]func(*Runtime), len(areas))
	for i, area := range areas {
		installers[i] = area.Install
	}
	return installers
}
