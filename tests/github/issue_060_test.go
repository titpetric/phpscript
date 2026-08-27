package github_test

import (
	"net/http"
	"testing"
)

// Test_Issue060 covers a default-value path parameter.
//
// https://github.com/titpetric/phpscript/issues/60
//
// The annotation registered a chi parameter literally named "module=users",
// which matched every request to the segment and exported nothing: a route
// that looks live and a parameter that never arrives. There is no
// default-value syntax, so the path is an authoring error and is not
// registered at all.
func Test_Issue060(t *testing.T) {
	routes := map[string]string{
		"bad.php": "// @route GET /a/{module=users}",
	}
	for name, serve := range routers(t, routes) {
		t.Run(name, func(t *testing.T) {
			status, body := get(t, serve, "/a/anything")
			if status != http.StatusNotFound {
				t.Fatalf("status = %d, body = %q, want 404", status, body)
			}
		})
	}
}

// The same path is reported by the linter, which is where an author sees it
// before a request ever reaches the route.
func Test_Issue060Lint(t *testing.T) {
	diags := lintRoutes(t, "<?php\n// @route GET /a/{module=users}\n")
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one", diags)
	}
	if diags[0].Line != 2 {
		t.Fatalf("line = %d, want 2", diags[0].Line)
	}
	want := "invalid route parameter {module=users}"
	if !contains(diags[0].Message, want) {
		t.Fatalf("message = %q, want it to contain %q", diags[0].Message, want)
	}
}
