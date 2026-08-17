package annotations_test

import (
	"net/http"
	"testing"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/model"
)

func TestParseRoutesReadsCommentAnnotations(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   []model.RouteAnnotation
	}{
		{
			name:   "method and path",
			source: "<?php\n// @route GET /users/{id}",
			want:   []model.RouteAnnotation{{Method: http.MethodGet, Path: "/users/{id}"}},
		},
		{
			name:   "path only defaults to GET and POST",
			source: "<?php\n// @route: /submit",
			want: []model.RouteAnnotation{
				{Method: http.MethodGet, Path: "/submit"},
				{Method: http.MethodPost, Path: "/submit"},
			},
		},
		{
			name:   "docblock",
			source: "<?php\n/**\n * @route PUT /users/{id}\n */",
			want:   []model.RouteAnnotation{{Method: http.MethodPut, Path: "/users/{id}"}},
		},
		{
			name:   "annotation inside a PHP string",
			source: `<?php echo "// @route GET /nope";`,
		},
		{
			name:   "tag without a path",
			source: "<?php\n// @route",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := annotations.ParseRoutes([]byte(test.source))
			if len(got) != len(test.want) {
				t.Fatalf("routes = %+v, want %+v", got, test.want)
			}
			for i, route := range got {
				if route != test.want[i] {
					t.Fatalf("routes[%d] = %+v, want %+v", i, route, test.want[i])
				}
			}
		})
	}
}

func TestHasStartupRequiresCommentAnnotation(t *testing.T) {
	if annotations.HasStartup([]byte(`<?php echo "// @startup";`)) {
		t.Fatal("annotation inside PHP string was detected")
	}
	if !annotations.HasStartup([]byte("<?php\n/**\n * @startup\n */")) {
		t.Fatal("docblock annotation was not detected")
	}
	if !annotations.HasStartup([]byte("<?php\n# @startup:")) {
		t.Fatal("hash annotation was not detected")
	}
}
