package annotations_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
)

var testHandlerFileSystem = fstest.MapFS{
	"sum.php": {Data: []byte(`<?php
// @route GET /sum
$total = 2 + 3;
echo $total * 4;
`)},
	"quiet.php": {Data: []byte(`<?php
// @route GET /quiet
http_response_code(410);
`)},
	"broken.php": {Data: []byte(`<?php
// @route GET /broken
throw new Exception("connection refused", 0);
`)},
}

// TestWithExprCache covers the compiled expressions a host shares across
// requests. The endpoints fill the cache a host passed, and the entries are
// keyed by AST node identity, so a second request reuses them only where the
// program it ran is the one the include cache already held. A tree reparsed
// per request keys every entry on fresh nodes and the cache grows.
func TestWithExprCache(t *testing.T) {
	cache := runner.NewExprCache()
	if got := cache.Len(); got != 0 {
		t.Fatalf("Len() = %d on a fresh cache", got)
	}

	mux := http.NewServeMux()
	route := annotations.NewRoute(testHandlerFileSystem,
		annotations.WithExprCache(cache),
		annotations.WithIncludeCache(runner.NewIncludeCache()),
		annotations.WithRunnerOptions(runner.Options{Precompile: true}),
	)
	if err := route.RegisterMux(mux); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sum", nil))
	if response.Code != http.StatusOK || response.Body.String() != "20" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}

	filled := cache.Len()
	if filled == 0 {
		t.Fatal("the endpoint compiled nothing into the supplied cache")
	}

	second := httptest.NewRecorder()
	mux.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/sum", nil))
	if second.Body.String() != "20" {
		t.Fatalf("body = %q on the second request", second.Body.String())
	}
	if got := cache.Len(); got != filled {
		t.Errorf("Len() = %d after a second request, want the %d entries the first one wrote", got, filled)
	}
}

// TestErrorPageFunc covers the two arguments a host page is called with. The
// status is what the endpoint ended on, and the notes are what went wrong,
// empty unless the endpoint failed: a 410 the script set is an answer the
// script chose, and a thrown exception is a failure whose message the host may
// render. The page reports whether it rendered, so a false answer leaves the
// response to the endpoint.
func TestErrorPageFunc(t *testing.T) {
	for _, test := range []struct {
		path   string
		status int
		notes  string
	}{
		{path: "/quiet", status: http.StatusGone},
		{path: "/broken", status: http.StatusInternalServerError, notes: "connection refused"},
	} {
		t.Run(test.path, func(t *testing.T) {
			var gotStatus int
			var gotNotes string
			page := annotations.WithErrorPages(func(w http.ResponseWriter, r *http.Request, status int, notes string) bool {
				gotStatus, gotNotes = status, notes
				if r.URL.Path != test.path {
					t.Errorf("the page was called for %q, want %q", r.URL.Path, test.path)
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("the page"))
				return true
			})

			mux := http.NewServeMux()
			if err := annotations.NewRoute(testHandlerFileSystem, page).RegisterMux(mux); err != nil {
				t.Fatal(err)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))

			if gotStatus != test.status {
				t.Errorf("status = %d, want %d", gotStatus, test.status)
			}
			if test.notes == "" && gotNotes != "" {
				t.Errorf("notes = %q, want nothing for a status the script chose", gotNotes)
			}
			if test.notes != "" && !strings.Contains(gotNotes, test.notes) {
				t.Errorf("notes = %q, want the failure message %q in it", gotNotes, test.notes)
			}
			if response.Code != test.status || response.Body.String() != "the page" {
				t.Errorf("response = %d %q, want the page the host rendered", response.Code, response.Body.String())
			}
		})
	}
}
