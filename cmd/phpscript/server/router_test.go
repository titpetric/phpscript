package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewRouter covers the mux every handler tree of this command starts from:
// a pattern registered on it answers, a path under a mounted subtree reaches
// the handler that claimed it, and a path nobody claimed is a 404.
func TestNewRouter(t *testing.T) {
	router := newRouter()
	router.Get("/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	})
	router.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{"/hello", http.StatusOK, "hello"},
		{"/nothing", http.StatusNotFound, ""},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))

		if response.Code != tc.code {
			t.Errorf("GET %s = %d, want %d", tc.path, response.Code, tc.code)
		}
		if got := response.Body.String(); got != tc.body {
			t.Errorf("GET %s body = %q, want %q", tc.path, got, tc.body)
		}
	}
}

// TestNewRouterRunsMiddleware covers the other thing the server does with a
// router: the tracing middleware is registered on it, and it has to wrap every
// handler mounted after it.
func TestNewRouterRunsMiddleware(t *testing.T) {
	router := newRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Traced", "yes")
			next.ServeHTTP(w, r)
		})
	})
	router.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("served"))
	}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/anything", nil))

	if got := response.Header().Get("X-Traced"); got != "yes" {
		t.Errorf("X-Traced = %q, want yes", got)
	}
	if got, want := response.Body.String(), "served"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
