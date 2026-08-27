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

	// The three parameter spellings, and one the grammar refuses.
	"params/rest.php": {Data: []byte(`<?php
// @route GET /files/{pathname...}
echo $_PATH["pathname"];
`)},
	"params/constrained.php": {Data: []byte(`<?php
// @route GET /orders/{id:[0-9]+}
echo $_PATH["id"];
`)},
	"params/malformed.php": {Data: []byte(`<?php
// @route GET /bad/{module=users}
echo "reached";
`)},

	// The four ways an endpoint can end an error, for the error page tests.
	"api/quiet.php": {Data: []byte(`<?php
// @route GET /api/quiet
http_response_code(404);
`)},
	"api/payload.php": {Data: []byte(`<?php
// @route GET /api/payload
http_response_code(404);
echo '{"error":"no such user"}';
`)},
	"api/declared.php": {Data: []byte(`<?php
// @route GET /api/declared
header("Content-Type: application/json");
http_response_code(404);
`)},
	"api/broken.php": {Data: []byte(`<?php
// @route GET /api/broken
throw new Exception("connection refused", 0);
`)},
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

// The three parameter spellings answer the same way on both routers, though
// neither router understands all three as written: chi 404s on {name...} and
// ServeMux panics on {name:regex}, so each is registered in its own dialect
// and $_PATH is keyed from the declared path.
//
// A path the grammar refuses is not registered at all. chi would take
// {module=users} as a parameter of that literal name, match every request to
// the segment and export nothing.
func TestRouteParameterSpellings(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		status int
		body   string
	}{
		{"one segment", "/users/42", http.StatusOK, "42"},
		{"remaining segments", "/files/docs/2026/report.pdf", http.StatusOK, "docs/2026/report.pdf"},
		{"remaining segments, one deep", "/files/report.pdf", http.StatusOK, "report.pdf"},
		{"constraint matches", "/orders/123", http.StatusOK, "123"},
		{"constraint refuses", "/orders/xyz", http.StatusNotFound, ""},
		{"malformed is not registered", "/bad/anything", http.StatusNotFound, ""},
	}

	routers := map[string]func(t *testing.T) http.Handler{
		"servemux": func(t *testing.T) http.Handler { return newTestMux(t) },
		"chi": func(t *testing.T) http.Handler {
			router := chi.NewRouter()
			if err := annotations.NewRoute(testRouteFileSystem).Mount(context.Background(), router); err != nil {
				t.Fatal(err)
			}
			return router
		},
	}
	for routerName, build := range routers {
		for _, test := range tests {
			t.Run(routerName+"/"+test.name, func(t *testing.T) {
				// ServeMux has no regex constraint, so it answers a request
				// chi refuses. The parameter still arrives.
				status, body := test.status, test.body
				if routerName == "servemux" && test.name == "constraint refuses" {
					status, body = http.StatusOK, "xyz"
				}
				rr := httptest.NewRecorder()
				build(t).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, test.url, nil))
				if rr.Code != status {
					t.Fatalf("status = %d, want %d, body = %q", rr.Code, status, rr.Body.String())
				}
				if status == http.StatusOK && rr.Body.String() != body {
					t.Fatalf("body = %q, want %q", rr.Body.String(), body)
				}
			})
		}
	}
}

// TestRouteErrorPagesAreOfferedOnlyAnUnansweredError pins what a host's error
// page is allowed to replace. An endpoint that put a payload in the body, or
// declared what it answers with, has answered, and its answer stands; an
// endpoint that set a status and said nothing else has left a hole a page may
// fill. None of it depends on the endpoint sitting under /api, which is the
// point: the routes here do, and it is not what decides.
func TestRouteErrorPagesAreOfferedOnlyAnUnansweredError(t *testing.T) {
	tests := []struct {
		path    string
		offered bool
		status  int
		body    string
	}{
		{path: "/api/quiet", offered: true, status: http.StatusNotFound, body: "the page"},
		{path: "/api/payload", offered: false, status: http.StatusNotFound, body: `{"error":"no such user"}`},
		{path: "/api/declared", offered: false, status: http.StatusNotFound, body: ""},
		{path: "/api/broken", offered: true, status: http.StatusInternalServerError, body: "the page"},
		// A 200 is not an error and is never offered.
		{path: "/users/42", offered: false, status: http.StatusOK, body: "42"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			var offered bool
			mux := newTestMux(t, annotations.WithErrorPages(
				func(w http.ResponseWriter, _ *http.Request, status int, _ string) bool {
					offered = true
					w.WriteHeader(status)
					_, _ = w.Write([]byte("the page"))
					return true
				},
			))

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, test.path, nil))

			if offered != test.offered {
				t.Fatalf("error page offered = %v, want %v", offered, test.offered)
			}
			if rr.Code != test.status {
				t.Fatalf("status = %d, want %d", rr.Code, test.status)
			}
			if rr.Body.String() != test.body {
				t.Fatalf("body = %q, want %q", rr.Body.String(), test.body)
			}
		})
	}
}

// TestRouteWithoutErrorPagesReportsOnlyTheStatus pins what a failed endpoint
// answers when no host page is configured: the status and its standard text.
// What went wrong is in the log and on the trace, not in a body the client
// reads back.
func TestRouteWithoutErrorPagesReportsOnlyTheStatus(t *testing.T) {
	mux := newTestMux(t)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/broken", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if rr.Body.String() != "Internal Server Error\n" {
		t.Fatalf("body = %q, want the status text alone", rr.Body.String())
	}
}

// TestRouteErrorPageDeclinesLeavesTheStatus pins that a host page which does
// not render, the ordinary case of a request no page is meant for, leaves the
// endpoint's own response as it was.
func TestRouteErrorPageDeclinesLeavesTheStatus(t *testing.T) {
	mux := newTestMux(t, annotations.WithErrorPages(
		func(http.ResponseWriter, *http.Request, int, string) bool { return false },
	))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/quiet", nil))
	if rr.Code != http.StatusNotFound || rr.Body.String() != "" {
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
