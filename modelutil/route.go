package modelutil

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// ParseRoutePath returns the parameters declared in an @route path, in order.
//
// An error names the offending segment and a caller can report it against the
// annotation's line. Every rejected spelling is one a router
// would otherwise accept and answer wrongly: chi registers {module=users} as a
// parameter of that literal name, matches every request to the segment and
// exports nothing.
func ParseRoutePath(path string) ([]model.RouteParam, error) {
	var params []model.RouteParam
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

		// {$} is ServeMux's end-of-path terminator and no parameter. The
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

// The type holds the name rule; the path is the location only this knows.
func parseRouteParam(body, path string) (model.RouteParam, error) {
	if body == "" {
		return model.RouteParam{}, fmt.Errorf("empty route parameter in route path %q", path)
	}

	param := model.RouteParam{Name: body}
	switch {
	case strings.Contains(body, ":"):
		name, pattern, _ := strings.Cut(body, ":")
		if pattern == "" {
			return model.RouteParam{}, fmt.Errorf("route parameter {%s} declares an empty pattern in route path %q", body, path)
		}
		param = model.RouteParam{Name: name, Pattern: pattern}
	case strings.HasSuffix(body, "..."):
		param = model.RouteParam{Name: strings.TrimSuffix(body, "..."), Rest: true}
	}

	if err := param.Valid(); err != nil {
		return model.RouteParam{}, fmt.Errorf("%w in route path %q", err, path)
	}
	return param, nil
}

// Braces nest, because a chi constraint holds its own: {id:[0-9]{3}}.
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

// RenderRoutePath rewrites an @route path into the dialect a router reads, and
// returns an unparsable path unchanged.
//
// chi takes {name...} as *, its only multi-segment placeholder; the rendered
// pattern loses the name and the runner reads the declared path for the values
// $_REQUEST holds. ServeMux has no regex syntax and drops the constraint.
func RenderRoutePath(path string, dialect model.RouteDialect) string {
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
		case param.Rest && dialect == model.RouteChi:
			b.WriteString("*")
		case param.Pattern != "" && dialect == model.RouteServeMux:
			b.WriteString("{" + param.Name + "}")
		default:
			b.WriteString("{" + body + "}")
		}
	}
	return b.String()
}
