package runner

import (
	"sync"
)

// bindings holds the runtime installers contributed by binding packages.
var bindings struct {
	sync.Mutex
	installers []func(*Runtime)
}

// RegisterBinding contributes a runtime installer, which stdlib.Register runs
// on every Runtime it sets up. Binding packages call it from their init() (see
// stdlib/ps/init.go), so a package is wired in by importing it — the way a
// program imports a database/sql driver. The registry lives here, in the leaf
// package that owns Runtime, so that stdlib can blank-import its subpackages
// without the two importing each other.
//
// Installers run in registration order, before the bindings a host passes to
// stdlib.Register directly.
func RegisterBinding(installer func(*Runtime)) {
	if installer == nil {
		return
	}
	bindings.Lock()
	defer bindings.Unlock()
	bindings.installers = append(bindings.installers, installer)
}

// Bindings returns the contributed installers in registration order.
func Bindings() []func(*Runtime) {
	bindings.Lock()
	defer bindings.Unlock()
	return append([]func(*Runtime){}, bindings.installers...)
}
