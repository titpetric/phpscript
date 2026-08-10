package route

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var testRouteFileSystem = fstest.MapFS{
	"index.php": {Data: []byte(`<?php
// @route GET /
echo "home";
`)},
	"users/show.php": {Data: []byte(`<?php
// @route GET /users/{id}
echo $_PATH["id"];
`)},
	"submit.php": {Data: []byte(`<?php
// @route: /submit
echo $_POST["name"];
`)},
	"redirect.php": {Data: []byte(`<?php
// @route POST /redirect
header("Location: /done");
exit;
`)},
	"public/annotated.php": {Data: []byte(`<?php
// @route GET /not-registered
echo "public";
`)},
	"ignored.txt": {Data: []byte(`// @route GET /ignored`)},
}

func TestServiceRegistersAnnotatedPHPFiles(t *testing.T) {
	mux := http.NewServeMux()
	svc, err := NewService(testRouteFileSystem, mux)
	if err != nil {
		t.Fatal(err)
	}
	if svc.mux != mux {
		t.Fatal("NewService did not use provided mux")
	}

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "42" {
		t.Fatalf("body = %q, want %q", got, "42")
	}
}

func TestRootRouteDoesNotMatchSubpaths(t *testing.T) {
	mux := http.NewServeMux()
	if _, err := NewService(testRouteFileSystem, mux); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/asset.css", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

func TestServiceRegistersDefaultGetAndPost(t *testing.T) {
	mux := http.NewServeMux()
	if _, err := NewService(testRouteFileSystem, mux); err != nil {
		t.Fatal(err)
	}

	body := "name=bob"
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "bob" {
		t.Fatalf("body = %q, want %q", got, "bob")
	}
}

func TestServiceAppliesRedirectStatus(t *testing.T) {
	mux := http.NewServeMux()
	if _, err := NewService(testRouteFileSystem, mux); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/redirect", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusFound)
	}
	if got := rr.Header().Get("Location"); got != "/done" {
		t.Fatalf("Location = %q, want /done", got)
	}
}

func TestServiceSkipsExcludedDirectory(t *testing.T) {
	mux := http.NewServeMux()
	if _, err := NewService(testRouteFileSystem, mux, WithExcludedDirectory("public")); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/not-registered", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}
