package annotations

import (
	"net/http"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// ParseRoutes returns the @route declarations found in src. A path-only
// annotation expands to both GET and POST, matching the HTTP router.
func ParseRoutes(src []byte) []model.RouteAnnotation {
	var routes []model.RouteAnnotation
	for i, line := range strings.Split(string(src), "\n") {
		text, ok := comment(line)
		if !ok {
			continue
		}
		name, fields := tag(text)
		if name != "@route" || len(fields) == 0 {
			continue
		}
		number := i + 1
		if len(fields) == 1 {
			routes = append(routes,
				model.RouteAnnotation{Method: http.MethodGet, Path: fields[0], Line: number},
				model.RouteAnnotation{Method: http.MethodPost, Path: fields[0], Line: number},
			)
			continue
		}
		routes = append(routes, model.RouteAnnotation{
			Method: strings.ToUpper(fields[0]),
			Path:   fields[1],
			Line:   number,
		})
	}
	return routes
}

// HasStartup reports whether src carries an @startup comment, marking a file
// the server executes once before it listens. `phpscript list` uses it to show
// startup files alongside routed ones.
func HasStartup(src []byte) bool {
	for line := range strings.SplitSeq(string(src), "\n") {
		text, ok := comment(line)
		if !ok {
			continue
		}
		if name, _ := tag(text); name == "@startup" {
			return true
		}
	}
	return false
}
