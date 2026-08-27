package github_test

import (
	"net/http"
	"testing"
)

// Test_Issue062 covers a path parameter matching the remaining segments.
//
// https://github.com/titpetric/phpscript/issues/62
//
// {tail...} registered and never matched, because nothing turned it into
// chi's wildcard, and the bare /* matched with no name to export the tail
// under. {tail...} is the one spelling; chi is handed * and publishes the
// value under "*", which the exporter maps back to the declared name.
func Test_Issue062(t *testing.T) {
	routes := map[string]string{
		"files.php": "// @route GET /c/{tail...}",
	}
	for name, serve := range routers(t, routes) {
		t.Run(name, func(t *testing.T) {
			// json_encode does not escape a forward slash here; php does.
			for _, test := range []struct{ url, want string }{
				{"/c/one/two", `{"tail":"one/two"}`},
				{"/c/one", `{"tail":"one"}`},
				{"/c/a/b/c/d", `{"tail":"a/b/c/d"}`},
			} {
				status, body := get(t, serve, test.url)
				if status != http.StatusOK || body != test.want {
					t.Fatalf("%s: status = %d, body = %q, want %q", test.url, status, body, test.want)
				}
			}
		})
	}
}
