package annotations_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
)

var testAnnotationsFileSystem = fstest.MapFS{
	"form.php": {Data: []byte(`<?php
// @route: /form
echo "form";
`)},
	"replace.php": {Data: []byte(`<?php
// @route PUT /replace
echo "replace";
`)},
}

// TestRoutedMethodsAreExplicit covers which requests reach an endpoint. A tag
// with no method routes GET and POST, so HEAD, OPTIONS and PUT are answered by
// the router and run no PHP. A method named on the tag is the only one that
// reaches that endpoint, so PUT is reachable where the author wrote it and
// nowhere else.
func TestRoutedMethodsAreExplicit(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	if err := annotations.NewRoute(testAnnotationsFileSystem).RegisterMux(mux); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		method string
		path   string
		status int
		body   string
	}{
		{method: http.MethodGet, path: "/form", status: http.StatusOK, body: "form"},
		{method: http.MethodPost, path: "/form", status: http.StatusOK, body: "form"},
		{method: http.MethodPut, path: "/form", status: http.StatusMethodNotAllowed},
		{method: http.MethodDelete, path: "/form", status: http.StatusMethodNotAllowed},
		{method: http.MethodPut, path: "/replace", status: http.StatusOK, body: "replace"},
		{method: http.MethodGet, path: "/replace", status: http.StatusMethodNotAllowed},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if test.body != "" && response.Body.String() != test.body {
				t.Errorf("body = %q, want %q", response.Body.String(), test.body)
			}
			if test.status == http.StatusMethodNotAllowed && response.Body.String() == test.body {
				t.Error("the endpoint ran for a method it never declared")
			}
		})
	}
}
