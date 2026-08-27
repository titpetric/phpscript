package model

import (
	"fmt"
	"strings"
)

// RouteParam is one {...} segment of an @route path.
//
// Three spellings are accepted, and they are the intersection of what the two
// routers this runtime registers on can express:
//
//	{name}        one path segment
//	{name...}     the remaining segments, joined
//	{name:regex}  one path segment matching regex
//
// Neither router accepts all three as written. chi has no {name...} and
// ServeMux panics on {name:regex}, so RenderPath translates per router and
// ParseRoutePath is the one grammar an author writes against.
type RouteParam struct {
	Name string

	// Rest is the {name...} form.
	Rest bool

	// Pattern is the regex of the {name:regex} form, empty otherwise. It is
	// enforced by chi and not by ServeMux, which has no equivalent.
	Pattern string
}

// RouteDialect names a router's pattern syntax.
type RouteDialect int

const (
	// RouteChi is go-chi/chi: {name} and {name:regex} are native, and the
	// remaining segments are the bare * wildcard, read as URL param "*".
	RouteChi RouteDialect = iota

	// RouteServeMux is net/http.ServeMux since Go 1.22: {name} and {name...}
	// are native, and a regex constraint has no equivalent.
	RouteServeMux
)

// ParseRoutePath returns the parameters declared in an @route path, in order.
//
// An error names the offending segment, so a caller can report it against the
// line the annotation was written on. Every rejected spelling is one a router
// would otherwise accept and answer wrongly: chi registers {module=users} as a
// parameter of that literal name, matches every request to the segment and
// exports nothing.
func ParseRoutePath(path string) ([]RouteParam, error) {
	var params []RouteParam
	seen := map[string]bool{}
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			continue
		}
		end := closingBrace(path, i)
		if end < 0 {
			return nil, fmt.Errorf("unclosed %q in route path %q", path[i:], path)
		}
		body := path[i+1 : end]
		i = end

		// {$} is ServeMux's end-of-path terminator, not a parameter. The
		// registrar writes it; an author can too.
		if body == "$" {
			continue
		}
		param, err := parseRouteParam(body, path)
		if err != nil {
			return nil, err
		}
		if seen[param.Name] {
			return nil, fmt.Errorf("duplicate route parameter %q in route path %q", param.Name, path)
		}
		seen[param.Name] = true
		params = append(params, param)
	}
	return params, nil
}

func parseRouteParam(body, path string) (RouteParam, error) {
	if body == "" {
		return RouteParam{}, fmt.Errorf("empty route parameter in route path %q", path)
	}
	if name, pattern, ok := strings.Cut(body, ":"); ok {
		if err := validRouteName(name, path); err != nil {
			return RouteParam{}, err
		}
		if pattern == "" {
			return RouteParam{}, fmt.Errorf("route parameter {%s} declares an empty pattern in route path %q", body, path)
		}
		return RouteParam{Name: name, Pattern: pattern}, nil
	}
	if name, ok := strings.CutSuffix(body, "..."); ok {
		if err := validRouteName(name, path); err != nil {
			return RouteParam{}, err
		}
		return RouteParam{Name: name, Rest: true}, nil
	}
	if err := validRouteName(body, path); err != nil {
		return RouteParam{}, err
	}
	return RouteParam{Name: body}, nil
}

// validRouteName accepts the names both routers accept and $_PATH can key on.
func validRouteName(name, path string) error {
	if name == "" {
		return fmt.Errorf("empty route parameter name in route path %q", path)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		alpha := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
		digit := c >= '0' && c <= '9'
		if !alpha && !(digit && i > 0) {
			return fmt.Errorf("invalid route parameter {%s} in route path %q: a name is letters, digits and underscores, "+
				"optionally followed by ... for the remaining segments or :pattern for a constraint", name, path)
		}
	}
	return nil
}

// closingBrace returns the index of the brace closing the one at open, or -1.
// Braces nest, because a chi constraint carries its own: {id:[0-9]{3}}.
func closingBrace(path string, open int) int {
	depth := 0
	for i := open; i < len(path); i++ {
		switch path[i] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// RenderRoutePath rewrites an @route path into the dialect a router reads.
//
// The one rewrite chi needs is {name...} to *, its only multi-segment
// placeholder; the name is not recoverable from the rendered pattern, so
// $_PATH is built from the declared path instead.
//
// ServeMux drops a regex constraint, because it has none: the parameter still
// matches its segment and the constraint is not enforced. RegisterMux says so.
//
// A path that does not parse is returned unchanged. Callers validate first.
func RenderRoutePath(path string, dialect RouteDialect) string {
	if _, err := ParseRoutePath(path); err != nil {
		return path
	}
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			b.WriteByte(path[i])
			continue
		}
		end := closingBrace(path, i)
		body := path[i+1 : end]
		i = end
		if body == "$" {
			b.WriteString("{$}")
			continue
		}
		param, _ := parseRouteParam(body, path)
		switch {
		case param.Rest && dialect == RouteChi:
			b.WriteString("*")
		case param.Pattern != "" && dialect == RouteServeMux:
			b.WriteString("{" + param.Name + "}")
		default:
			b.WriteString("{" + body + "}")
		}
	}
	return b.String()
}
