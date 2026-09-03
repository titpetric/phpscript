// Command plugin measures what it costs to cross from PHP into a Go plugin.
//
// It registers Bench, whose methods do as little as a method can, so a
// benchmark loop measures the crossing rather than the work behind it. The
// number it produces is compared against the same measurement through a
// compiled-in binding: a plugin symbol is an ordinary function value once
// dlopen has run, so the difference should be nothing, and this plugin is how
// that is checked rather than assumed.
//
// Build:
//
//	CGO_ENABLED=1 go build -buildmode=plugin -o plugin.so .
//
// The build passes neither -trimpath nor -ldflags, because neither `go test`
// nor `go install .` does, and a plugin has to match the binary that opens it.
//
// Nothing here imports phpscript. Host is declared below and satisfied
// structurally, so this plugin keeps working across a phpscript rebuild.
package main

import (
	"context"
	"log"
)

// Host is what this plugin needs from the runtime. Go compares interface types
// structurally, so *runner.Runtime satisfies this without either side knowing
// the other's name, and this file links no phpscript package.
type Host interface {
	RegisterConstructor(name string, ctor any)
}

// Init runs once per process. This plugin has no process state, so it has
// nothing to do; the entry point is still required, because a missing symbol
// should be a mistake rather than an implied default.
func Init(ctx context.Context) error { return nil }

// Bind installs Bench on the runtime serving the current request.
//
// It registers a constructor and no functions, deliberately. RegisterConstructor
// writes a map entry; RegisterFunc bumps the runtime's function generation,
// which invalidates its expression type environment, its compile configuration
// and every pooled evaluation environment. A per-request Bind that called
// RegisterFunc would undo the runtime's caching on every request, which is
// exactly what this plugin exists to measure.
func Bind(ctx context.Context, h Host) error {
	h.RegisterConstructor("Bench", func() *Bench { return &Bench{} })
	return nil
}

// Bench is the PHP-visible measurement subject.
type Bench struct {
	n int64
}

// Nop returns nothing and does nothing. It is the cheapest crossing there is,
// so it prices the bridge with the callee subtracted.
func (b *Bench) Nop() {}

// Count increments and returns the new total. A loop calling it cannot be
// optimised away, and the total is what a fixture asserts to prove every
// iteration actually reached the plugin.
func (b *Bench) Count() int64 {
	b.n++
	return b.n
}

// Echo returns its argument: one string in, one string out, which is the
// cheapest shape that moves a value in both directions.
func (b *Bench) Echo(value string) string { return value }

// Report writes a measured cost to the log, which goes to stderr. A fixture's
// output is compared exactly, so a timing cannot be printed to it; stderr is
// how the figure reaches a human running the fixture, while the fixture itself
// asserts only properties that do not vary between runs.
func (b *Bench) Report(ops int64, seconds float64) {
	if ops <= 0 {
		return
	}
	log.Printf("plugin/basic: %d crossings, %.0f ns/op", ops, seconds*1e9/float64(ops))
}
