// Package expr is the runner's expression engine: model.Expr compiles to a
// chain of typed Go closures (direct.go) that call the PHP-semantic helpers
// directly and read variables by slot from the Env carrier. It grew as the
// seam in front of github.com/expr-lang/expr and replaced it engine-first:
// the closure-chain technique is guamoko995/expr-cls's, applied to the
// model AST once the transpile round-trip had nothing left to do.
package expr

// Program is one compiled expression: a closure chain and the slot table its
// per-evaluation identifiers resolve through.
type Program struct {
	run   func(env *Env) (any, error)
	slots map[string]int
}

// Run evaluates a compiled program against the runner's environment.
func Run(p *Program, env *Env) (any, error) {
	return p.run(env)
}

// Slots reports the slot index assigned to each per-evaluation identifier,
// keyed by identifier. The runner resolves its variable list against it once
// per compiled expression; the map is read-only after compilation.
func (p *Program) Slots() map[string]int {
	return p.slots
}

// NumSlots is the size of the Vars slice Run expects.
func (p *Program) NumSlots() int {
	return len(p.slots)
}
