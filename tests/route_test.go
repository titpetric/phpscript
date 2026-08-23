package tests

import (
	"bytes"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps"
)

func TestRouteSharedMemoryFixture(t *testing.T) {
	root, err := fs.Sub(fixturesFS, "fixtures/routing")
	if err != nil {
		t.Fatal(err)
	}

	shm := ps.NewSharedMemory()
	mux := http.NewServeMux()
	err = annotations.NewRoute(root, annotations.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.SetContext(ps.SharedMemoryContext(rt.Context(), shm))
		rt.RegisterConstructor("SharedMemory", ps.NewSharedMemoryBinding)
	})).RegisterMux(mux)
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

// TestRouteFileUpload drives a file upload through the routed handler, the path
// a browser form takes: the multipart body has to reach both superglobals, and
// the temporary copy has to be gone once the response is written.
func TestRouteFileUpload(t *testing.T) {
	root, err := fs.Sub(fixturesFS, "fixtures/routing")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	if err := annotations.NewRoute(root).RegisterMux(mux); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("title", "q3"); err != nil {
		t.Fatal(err)
	}
	part, err := w.CreateFormFile("report", "report.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("a,b,c")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	fields := strings.Split(rr.Body.String(), "|")
	if len(fields) != 5 {
		t.Fatalf("body = %q, want 5 fields", rr.Body.String())
	}
	if got, want := strings.Join(fields[:4], "|"), "q3|report.csv|5|0"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(fields[4]); !os.IsNotExist(err) {
		t.Fatalf("tmp_name %q after response: err = %v, want not-exist", fields[4], err)
	}
}

func Example_routeSharedMemory() {
	root, err := fs.Sub(fixturesFS, "fixtures/routing")
	if err != nil {
		fmt.Println(err)
		return
	}

	shm := ps.NewSharedMemory()
	mux := http.NewServeMux()
	err = annotations.NewRoute(root, annotations.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.SetContext(ps.SharedMemoryContext(rt.Context(), shm))
		rt.RegisterConstructor("SharedMemory", ps.NewSharedMemoryBinding)
	})).RegisterMux(mux)
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
