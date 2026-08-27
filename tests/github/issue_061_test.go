package github_test

import (
	"net/http"
	"testing"
)

// Test_Issue061 covers a regex-constrained path parameter.
//
// https://github.com/titpetric/phpscript/issues/61
//
// chi enforced {id:[0-9]+} and carried the value, and the exporter recognised
// {name} and {name...} only, so $_PATH arrived empty.
//
// ServeMux has no regex constraint. It is registered with the bare parameter
// and answers a request chi refuses, which is why the two routers differ on
// the last case here.
func Test_Issue061(t *testing.T) {
	routes := map[string]string{
		"orders.php": "// @route GET /b/{id:[0-9]+}",
	}
	for name, serve := range routers(t, routes) {
		t.Run(name, func(t *testing.T) {
			status, body := get(t, serve, "/b/123")
			if status != http.StatusOK || body != `{"id":"123"}` {
				t.Fatalf("status = %d, body = %q", status, body)
			}

			status, body = get(t, serve, "/b/xyz")
			switch name {
			case "chi":
				if status != http.StatusNotFound {
					t.Fatalf("status = %d, body = %q, want 404", status, body)
				}
			case "servemux":
				if status != http.StatusOK || body != `{"id":"xyz"}` {
					t.Fatalf("status = %d, body = %q", status, body)
				}
			}
		})
	}
}
