package runner

import (
	"context"

	"github.com/titpetric/phpscript/telemetry"
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
}

// NewScope returns an empty scope.
func NewScope() *Scope {
	return &Scope{vars: map[string]any{}}
}

// Get returns the value of name and whether it is set.
func (s *Scope) Get(name string) (any, bool) {
	v, ok := s.vars[name]
	return v, ok
}

// Set stores name=val.
func (s *Scope) Set(name string, val any) {
	s.vars[name] = val
}

// Unset removes name from the frame (PHP's unset). Removing a name that was
// never set is not an error.
func (s *Scope) Unset(name string) {
	delete(s.vars, name)
}

// Defer registers a callback to run when the current PHP execution frame
// returns. Callbacks run in last-in, first-out order.
func (s *Scope) Defer(callback any) {
	s.deferred = append(s.deferred, callback)
}

// DefinedVars returns a snapshot of PHP-visible variables in this frame.
// Interpreter bookkeeping slots use a double-underscore prefix and are not PHP
// variables, so they are omitted.
func (s *Scope) DefinedVars() map[string]any {
	vars := make(map[string]any, len(s.vars))
	for name, value := range s.vars {
		if len(name) >= 2 && name[:2] == "__" {
			continue
		}
		vars[name] = value
	}
	return vars
}

func contextWithScope(ctx context.Context, scope *Scope) context.Context {
	if filename, ok := scope.Get("__FILE__"); ok {
		if filename, ok := filename.(string); ok {
			ctx = telemetry.WithSpanFilename(ctx, filename)
		}
	}
	if line, ok := scope.Get("__LINE__"); ok {
		if line, ok := line.(int); ok {
			ctx = telemetry.WithSpanLine(ctx, line)
		}
	}
	return context.WithValue(ctx, scopeContextKey{}, scope)
}

func contextWithEnv(ctx context.Context, env map[string]string) context.Context {
	return context.WithValue(ctx, envContextKey{}, env)
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
