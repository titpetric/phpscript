package tests

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/route"
	"github.com/titpetric/phpscript/runner"
)

func TestRouteSharedMemoryFixture(t *testing.T) {
	root, err := fs.Sub(fixturesFS, "fixtures/route")
	if err != nil {
		t.Fatal(err)
	}

	shm := NewSharedMemory()
	mux := http.NewServeMux()
	_, err = route.NewService(root, mux, route.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.SetContext(SharedMemoryContext(rt.Context(), shm))
		rt.RegisterConstructor("SharedMemory", NewSharedMemoryBinding)
	}))
	if err != nil {
		t.Fatal(err)
	}

	post := httptest.NewRequest(http.MethodPost, "/kv/color", strings.NewReader("value=blue"))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, post)
	if postRR.Code != http.StatusOK || postRR.Body.String() != "ok" {
		t.Fatalf("POST status/body = %d/%q", postRR.Code, postRR.Body.String())
	}

	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/kv/color", nil))
	if getRR.Code != http.StatusOK || getRR.Body.String() != "blue" {
		t.Fatalf("GET status/body = %d/%q", getRR.Code, getRR.Body.String())
	}

	statsRR := httptest.NewRecorder()
	mux.ServeHTTP(statsRR, httptest.NewRequest(http.MethodGet, "/stats/requests", nil))
	if statsRR.Code != http.StatusOK || statsRR.Body.String() != "2" {
		t.Fatalf("stats status/body = %d/%q", statsRR.Code, statsRR.Body.String())
	}
}

func ExampleSharedMemory() {
	root, err := fs.Sub(fixturesFS, "fixtures/route")
	if err != nil {
		fmt.Println(err)
		return
	}

	shm := NewSharedMemory()
	mux := http.NewServeMux()
	_, err = route.NewService(root, mux, route.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.SetContext(SharedMemoryContext(rt.Context(), shm))
		rt.RegisterConstructor("SharedMemory", NewSharedMemoryBinding)
	}))
	if err != nil {
		fmt.Println(err)
		return
	}

	post := httptest.NewRequest(http.MethodPost, "/kv/color", strings.NewReader("value=blue"))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, post)
	fmt.Println(postRR.Body.String())

	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/kv/color", nil))
	fmt.Println(getRR.Body.String())

	statsRR := httptest.NewRecorder()
	mux.ServeHTTP(statsRR, httptest.NewRequest(http.MethodGet, "/stats/requests", nil))
	fmt.Println(statsRR.Body.String())

	// Output:
	// ok
	// blue
	// 2
}
