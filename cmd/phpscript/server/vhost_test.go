package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeHost(t *testing.T) {
	for _, test := range []struct {
		host string
		want string
	}{
		{host: "example.com", want: "example.com"},
		{host: "Example.COM", want: "example.com"},
		{host: "example.com:8080", want: "example.com"},
		{host: "example.com.", want: "example.com"},
		{host: "Example.COM.:8080", want: "example.com"},
		{host: " example.com ", want: "example.com"},
		{host: "127.0.0.1:8080", want: "127.0.0.1"},
		{host: "[::1]:8080", want: "::1"},
		{host: "[::1]", want: "::1"},
		{host: "::1", want: "::1"},
		{host: "", want: ""},
	} {
		t.Run(test.host, func(t *testing.T) {
			if got := normalizeHost(test.host); got != test.want {
				t.Fatalf("normalizeHost(%q) = %q, want %q", test.host, got, test.want)
			}
		})
	}
}

// TestHostMuxRoutesByHost pins the contract the shared execution environment
// rests on: a request reaches the site that claims its Host, and nothing else.
func TestHostMuxRoutesByHost(t *testing.T) {
	mux := newHostMux(map[string]http.Handler{
		"shop.example.com": named("shop"),
		"Blog.Example.COM": named("blog"),
	})

	for _, test := range []struct {
		host string
		want string
		code int
	}{
		{host: "shop.example.com", want: "shop", code: http.StatusOK},
		{host: "SHOP.example.com:8080", want: "shop", code: http.StatusOK},
		{host: "blog.example.com", want: "blog", code: http.StatusOK},
		{host: "other.example.com", code: http.StatusNotFound},
		{host: "", code: http.StatusNotFound},
	} {
		t.Run(test.host, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Host = test.host
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != test.code {
				t.Fatalf("status = %d, want %d", response.Code, test.code)
			}
			if test.want != "" && response.Body.String() != test.want {
				t.Fatalf("body = %q, want %q", response.Body.String(), test.want)
			}
		})
	}
}

func named(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(name))
	})
}
