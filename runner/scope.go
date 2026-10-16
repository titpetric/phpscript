package runner

import (
	"context"
)

type scopeContextKey struct{}

type envContextKey struct{}

// Scope is a flat variable table for one execution frame.
//
// PHP has no block scoping: variables introduced inside if/for/foreach bodies
// live in the enclosing function scope. Each function call gets a fresh Scope;
// the file body runs in the global Scope.
//
// There is intentionally no `global` keyword implemented.
type Scope struct {
	vars     map[string]any
	deferred []any

	// args holds the current call's positional arguments for
	// func_get_args(). A typed field and no vars entry: storing the
	// slice under a hidden name boxed it into an interface on every call,
	// and the map write adds a second cost on the same hot path.
	args []any

	// statics maps a name declared by a `static $x` statement in this frame to
	// the persistent bag holding it (see Runtime.funcStatics). Reads and
	// writes of a bound name resolve through the bag, so a later
	// `$x = ...` in the function persists across calls. Nil in every frame
	// that declares no statics, so the common path pays one nil check.
	statics map[string]map[string]any
}

// NewScope returns an empty scope.
func NewScope() *Scope {
	return &Scope{vars: map[string]any{}}
}

// bindStatic links name to its persistent storage for the rest of this frame.
func (s *Scope) bindStatic(name string, bag map[string]any) {
	if s.statics == nil {
		s.statics = map[string]map[string]any{}
	}
	s.statics[name] = bag
}

// Get returns the value of name and whether it is set.
func (s *Scope) Get(name string) (any, bool) {
	if s.statics != nil {
		if bag, ok := s.statics[name]; ok {
			v, ok := bag[name]
			return v, ok
		}
	}
	v, ok := s.vars[name]
	return v, ok
}

// Set stores name=val.
func (s *Scope) Set(name string, val any) {
	if s.statics != nil {
		if bag, ok := s.statics[name]; ok {
			bag[name] = val
			return
		}
	}
	s.vars[name] = val
}

// Unset removes name from the frame (PHP's unset). Removing a name nothing set
// is not an error. Unsetting a static-bound name breaks the local
// link and leaves the stored value for the next call, as PHP does.
func (s *Scope) Unset(name string) {
	delete(s.statics, name)
	delete(s.vars, name)
}

// Defer registers a callback to run when the current PHP execution frame
// returns. Callbacks run in last-in, first-out order.
func (s *Scope) Defer(callback any) {
	s.deferred = append(s.deferred, callback)
}

// DefinedVars returns a snapshot of PHP-visible variables in this frame.
// Interpreter bookkeeping slots use a double-underscore prefix and are not PHP
// variables, so they are omitted. `this` goes with them: it is bound by the
// call and never declared in the body, and php leaves it out of
// get_defined_vars() for that reason.
func (s *Scope) DefinedVars() map[string]any {
	vars := make(map[string]any, len(s.vars)+len(s.statics))
	for name, value := range s.vars {
		if name == "this" || (len(name) >= 2 && name[:2] == "__") {
			continue
		}
		vars[name] = value
	}
	for name, bag := range s.statics {
		if value, ok := bag[name]; ok {
			vars[name] = value
		}
	}
	return vars
}

// contextWithScope carries to a span the calling frame, and the file and line
// to attribute it to, as the fields of one bindingCtx in place of three
// context.WithValue layers. Bindings take rt.bindingContext, which picks the
// request as the base while one is live; this one keeps the base the caller
// chose, for the trace region that must stay on the session chain.
func (rt *Runtime) contextWithScope(ctx context.Context, scope *Scope) context.Context {
	c := &bindingCtx{Context: ctx, scope: scope, env: rt.Env}
	rt.fillSource(scope, c)
	return c
}

// ScopeFromContext returns the active PHP execution frame attached to a
// context auto-injected into a registered free function.
func ScopeFromContext(ctx context.Context) (*Scope, bool) {
	scope, ok := ctx.Value(scopeContextKey{}).(*Scope)
	return scope, ok
}

// EnvFromContext returns the request-scoped environment attached to contexts
// auto-injected into registered Go callables.
func EnvFromContext(ctx context.Context) (map[string]string, bool) {
	env, ok := ctx.Value(envContextKey{}).(map[string]string)
	return env, ok
}
