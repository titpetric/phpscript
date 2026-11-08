package model_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestRouteParam_Valid covers the name rule both routers hold to, read off a
// parameter a caller filled.
//
// The error names the parameter and no path: modelutil.ParseRoutePath wraps it
// with the path it read, and a caller filling the struct read no path.
func TestRouteParam_Valid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		param model.RouteParam
		want  string
	}{
		{"a plain name", model.RouteParam{Name: "id"}, ""},
		{"an underscore", model.RouteParam{Name: "_id"}, ""},
		{"digits after the first byte", model.RouteParam{Name: "a1"}, ""},
		{"the remaining segments", model.RouteParam{Name: "rest", Rest: true}, ""},
		{"a constraint", model.RouteParam{Name: "id", Pattern: "[0-9]+"}, ""},
		{"no name", model.RouteParam{}, "empty route parameter name"},
		{"a leading digit", model.RouteParam{Name: "1a"}, "invalid route parameter"},
		{"an equals sign", model.RouteParam{Name: "module=users"}, "invalid route parameter"},
		{"a dash", model.RouteParam{Name: "user-id"}, "invalid route parameter"},
		{"both forms at once", model.RouteParam{Name: "rest", Rest: true, Pattern: "[0-9]+"}, "declares both"},
	} {
		err := tc.param.Valid()
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: Valid() = %v, want nil", tc.name, err)
		case tc.want != "" && err == nil:
			t.Errorf("%s: Valid() = nil, want an error naming %q", tc.name, tc.want)
		case tc.want != "" && !strings.Contains(err.Error(), tc.want):
			t.Errorf("%s: Valid() = %v, want it to name %q", tc.name, err, tc.want)
		}
	}
}

// TestRouteDialect covers the two dialects naming distinct values, which the
// default arm of a render switch depends on.
func TestRouteDialect(t *testing.T) {
	t.Parallel()
	if model.RouteChi == model.RouteServeMux {
		t.Fatal("the two dialects name one value")
	}
}
