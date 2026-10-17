package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
)

// TestPooledHandlerIsolatesRequests drives two requests through one handler
// and asserts the second starts on a reset runtime: the first request's
// declarations, constants and globals are gone, and its response still came
// out whole. The pool is sized to one so both requests take the same runtime.
func TestPooledHandlerIsolatesRequests(t *testing.T) {
	t.Parallel()

	tree := fstest.MapFS{
		"public/first.php": {Data: []byte(`<?php
define("REQUEST_ONE", 1);
function request_one() { return "first"; }
$leak = "secret";
echo request_one();`)},
		"public/second.php": {Data: []byte(`<?php
echo defined("REQUEST_ONE") || function_exists("request_one") || isset($leak) ? "leak" : "clean";`)},
	}

	h, err := newHandler(tree, "", DefaultDocumentRoot, runner.Options{}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	h.pool = runner.NewSessionPool(1, h.buildRuntime)

	serve := func(path string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		return rec.Body.String()
	}

	if got := serve("/first.php"); got != "first" {
		t.Fatalf("first request answered %q, want %q", got, "first")
	}
	if got := serve("/second.php"); got != "clean" {
		t.Fatalf("second request answered %q, want %q: the pooled runtime carried the first request's state", got, "clean")
	}
}
