package model_test

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

func TestParseRoutePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []model.RouteParam
	}{
		{"no parameters", "/health", nil},
		{"one segment", "/a/{id}", []model.RouteParam{{Name: "id"}}},
		{"remaining segments", "/c/{tail...}", []model.RouteParam{{Name: "tail", Rest: true}}},
		{"constraint", "/b/{id:[0-9]+}", []model.RouteParam{{Name: "id", Pattern: "[0-9]+"}}},
		{"braces inside a constraint", "/b/{id:[0-9]{3}}", []model.RouteParam{{Name: "id", Pattern: "[0-9]{3}"}}},
		{"several", "/{a}/{b}/{c...}", []model.RouteParam{{Name: "a"}, {Name: "b"}, {Name: "c", Rest: true}}},
		{"two in one segment", "/{month}-{day}", []model.RouteParam{{Name: "month"}, {Name: "day"}}},
		{"terminator is not a parameter", "/posts/{$}", nil},
		{"underscores and digits", "/{user_id2}", []model.RouteParam{{Name: "user_id2"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := model.ParseRoutePath(test.path)
			if err != nil {
				t.Fatalf("ParseRoutePath(%q): %v", test.path, err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("got %+v, want %+v", got, test.want)
				}
			}
		})
	}
}

// Every rejected spelling is one a router would otherwise accept and answer
// wrongly. chi registers {module=users} as a parameter of that literal name,
// matches every request to the segment and exports nothing.
func TestParseRoutePathRejects(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"default value", "/a/{module=users}", "invalid route parameter"},
		{"space in name", "/a/{user id}", "invalid route parameter"},
		{"dash in name", "/a/{user-id}", "invalid route parameter"},
		{"leading digit", "/a/{1st}", "invalid route parameter"},
		{"empty", "/a/{}", "empty route parameter"},
		{"empty name with pattern", "/a/{:[0-9]+}", "empty route parameter name"},
		{"empty pattern", "/a/{id:}", "empty pattern"},
		{"unclosed", "/a/{id", "unclosed"},
		{"duplicate name", "/a/{id}/b/{id}", "duplicate route parameter"},
		{"rest with a bad name", "/a/{tail-x...}", "invalid route parameter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := model.ParseRoutePath(test.path)
			if err == nil {
				t.Fatalf("ParseRoutePath(%q) accepted it", test.path)
			}
			if !contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want it to contain %q", err, test.want)
			}
		})
	}
}

// The rendered pattern is what each router is handed. The expectations were
// measured against chi v5.3.2 and net/http.ServeMux: chi 404s on {tail...} and
// ServeMux panics on {id:[0-9]+}, so neither dialect is the authored one.
func TestRenderRoutePath(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		chi   string
		serve string
	}{
		{"plain", "/a/{id}", "/a/{id}", "/a/{id}"},
		{"rest", "/c/{tail...}", "/c/*", "/c/{tail...}"},
		{"constraint", "/b/{id:[0-9]+}", "/b/{id:[0-9]+}", "/b/{id}"},
		{"braces in a constraint", "/b/{id:[0-9]{3}}", "/b/{id:[0-9]{3}}", "/b/{id}"},
		{"literal", "/health", "/health", "/health"},
		{"terminator", "/posts/{$}", "/posts/{$}", "/posts/{$}"},
		{"mixed", "/{a}/{b:[0-9]+}/{c...}", "/{a}/{b:[0-9]+}/*", "/{a}/{b}/{c...}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := model.RenderRoutePath(test.path, model.RouteChi); got != test.chi {
				t.Errorf("chi: got %q, want %q", got, test.chi)
			}
			if got := model.RenderRoutePath(test.path, model.RouteServeMux); got != test.serve {
				t.Errorf("servemux: got %q, want %q", got, test.serve)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
