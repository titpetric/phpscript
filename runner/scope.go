package runner

// Scope is a flat variable table for one execution frame.
//
// PHP has no block scoping: variables introduced inside if/for/foreach bodies
// live in the enclosing function scope. Each function call gets a fresh Scope;
// the file body runs in the global Scope.
//
// There is intentionally no `global` keyword implemented.
type Scope struct {
	vars map[string]any
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
