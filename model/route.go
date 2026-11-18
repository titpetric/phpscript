package model

import (
	"errors"
	"fmt"
)

// RouteParam holds one {...} segment of an @route path.
//
// modelutil.ParseRoutePath accepts three spellings: {name} for one segment,
// {name...} for the rest joined, {name:regex} for a regex match. Neither router
// takes all three as written; RenderRoutePath translates per router. An author
// writes against this grammar.
type RouteParam struct {
	Name string

	// Rest is the {name...} form.
	Rest bool

	// Pattern is the regex of the {name:regex} form, empty otherwise. chi
	// enforces it; ServeMux has no such syntax and ignores it.
	Pattern string
}

// Valid reports the name both routers accept and $_REQUEST can key. An error
// names the offending parameter.
//
// A caller holding the path the parameter came from wraps the error with it.
func (p RouteParam) Valid() error {
	if p.Name == "" {
		return errors.New("empty route parameter name")
	}
	for i := 0; i < len(p.Name); i++ {
		c := p.Name[i]
		alpha := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
		digit := c >= '0' && c <= '9'
		if !alpha && !(digit && i > 0) {
			return fmt.Errorf("invalid route parameter {%s}: a name is letters, digits and underscores, "+
				"optionally followed by ... for the remaining segments or :pattern for a constraint", p.Name)
		}
	}
	if p.Rest && p.Pattern != "" {
		return fmt.Errorf("route parameter {%s} declares both the remaining segments and a pattern", p.Name)
	}
	return nil
}

// RouteDialect names a router's pattern syntax.
type RouteDialect int

// The dialects modelutil.RenderRoutePath translates an @route path into.
const (
	// RouteChi is go-chi/chi: {name} and {name:regex} are native, and the
	// remaining segments are the bare * wildcard, read as URL param "*".
	RouteChi RouteDialect = iota

	// RouteServeMux is net/http.ServeMux since Go 1.22: {name} and {name...} are native.
	RouteServeMux
)
