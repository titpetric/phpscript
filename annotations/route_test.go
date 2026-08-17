package annotations_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	chi "github.com/go-chi/chi/v5"

	"github.com/titpetric/phpscript/annotations"
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

func newTestMux(t *testing.T, options ...annotations.Option) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if err := annotations.NewRoute(testRouteFileSystem, options...).RegisterMux(mux); err != nil {
		t.Fatal(err)
	}
	return mux
}

func TestRouteRegistersAnnotatedPHPFiles(t *testing.T) {
	mux := newTestMux(t)

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
	mux := newTestMux(t)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/asset.css", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

func TestRouteRegistersDefaultGetAndPost(t *testing.T) {
	mux := newTestMux(t)

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

func TestRouteAppliesRedirectStatus(t *testing.T) {
	mux := newTestMux(t)

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

func TestRouteSkipsExcludedDirectory(t *testing.T) {
	mux := newTestMux(t, annotations.WithExcludedDirectory("public"))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/not-registered", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

// TestRouteMountsOnPlatformRouter pins the second registrar: the same
// annotations reach a platform router as a lifecycle module.
func TestRouteMountsOnPlatformRouter(t *testing.T) {
	router := chi.NewRouter()
	if err := annotations.NewRoute(testRouteFileSystem).Mount(context.Background(), router); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "42" {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

func TestRouteRejectsMissingDestinations(t *testing.T) {
	route := annotations.NewRoute(testRouteFileSystem)
	if err := route.RegisterMux(nil); err == nil {
		t.Fatal("nil mux was accepted")
	}
	if err := annotations.NewRoute(nil).RegisterMux(http.NewServeMux()); err == nil {
		t.Fatal("nil root filesystem was accepted")
	}
}
