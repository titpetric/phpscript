package annotations_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
)

// recordingRegistrar is a router that records every registration. A
// Registrar is all Route needs to publish its endpoints, so this stands for a
// host router without implementing one.
type recordingRegistrar struct {
	handled  []string
	handlers map[string]http.Handler
}

// Handle records one registration, keeping the last handler for a pattern.
func (r *recordingRegistrar) Handle(method string, path string, handler http.Handler) {
	if r.handlers == nil {
		r.handlers = map[string]http.Handler{}
	}
	pattern := method + " " + path
	r.handled = append(r.handled, pattern)
	r.handlers[pattern] = handler
}

// body serves one recorded pattern and returns what the endpoint wrote.
func (r *recordingRegistrar) body(t *testing.T, pattern string) string {
	t.Helper()

	handler, ok := r.handlers[pattern]
	if !ok {
		t.Fatalf("%q was not registered, got %v", pattern, r.handled)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	return response.Body.String()
}

var testRegistrarFileSystem = fstest.MapFS{
	"a_duplicate.php": {Data: []byte(`<?php
// @route GET /duplicate
echo "first";
`)},
	"refused.php": {Data: []byte(`<?php
// @route GET /files/{module=users}
echo "refused";
`)},
	"users/show.php": {Data: []byte(`<?php
// @route GET /users/{id}
// @route POST /users/{id}
echo "user";
`)},
	"z_duplicate.php": {Data: []byte(`<?php
// @route GET /duplicate
echo "last";
`)},
}

// TestRegistrar covers the one type discovery and an application's router
// share: a method, a path and a handler. The path arrives as the annotation
// declared it, before either router's rewriting, so a host that implements
// this interface renders the parameters its own way. A repeated tag registers
// once per line, so one file answers two methods.
func TestRegistrar(t *testing.T) {
	t.Parallel()
	registrar := &recordingRegistrar{}
	if err := annotations.NewRoute(testRegistrarFileSystem).Register(registrar); err != nil {
		t.Fatal(err)
	}

	got := slices.Clone(registrar.handled)
	slices.Sort(got)
	want := []string{
		"GET /duplicate",
		"GET /duplicate",
		"GET /users/{id}",
		"POST /users/{id}",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("registered %v, want %v", got, want)
	}

	if body := registrar.body(t, "GET /users/{id}"); body != "user" {
		t.Errorf("the handler wrote %q, want the output of the file the tag was in", body)
	}
}

// TestRoute_Register covers the walk that publishes a tree. The walk passes
// over a path the route grammar refuses and registers none: chi registers
// {module=users} as a parameter of that literal name, matches every request to
// the segment and exports nothing, so a refused path would be a live route
// answering wrongly. Two files declaring one pattern both reach the registrar
// in path order, so the later file is the handler a router keeps.
func TestRoute_Register(t *testing.T) {
	t.Parallel()
	registrar := &recordingRegistrar{}
	if err := annotations.NewRoute(testRegistrarFileSystem).Register(registrar); err != nil {
		t.Fatal(err)
	}

	for _, pattern := range registrar.handled {
		if pattern == "GET /files/{module=users}" {
			t.Errorf("a path the grammar refuses was registered: %v", registrar.handled)
		}
	}
	if body := registrar.body(t, "GET /duplicate"); body != "last" {
		t.Errorf("the duplicate pattern kept %q, want the file registered last", body)
	}

	if err := annotations.NewRoute(testRegistrarFileSystem).Register(nil); err == nil {
		t.Error("a nil registrar was accepted, so the routes went nowhere")
	}
}
