// Package bindings holds the runtime's own PHP function bindings: the ones
// that answer for the request the runtime is serving, and the ones that need
// nothing but the Go standard library.
//
// It is a package below runner rather than beside the others under stdlib
// because it imports runner, which the reverse would make a cycle. Each file
// contributes its installer through runner.RegisterBinding in an init(), the
// way every stdlib package does, and stdlib/imports.go blank-imports this one
// so a binary that pulls stdlib gets these too.
package bindings
