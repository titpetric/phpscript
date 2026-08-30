package tests

import (
	"bytes"
	"encoding/json"
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
	"github.com/titpetric/phpscript/stdlib/core"
)

func TestRouteSharedMemoryFixture(t *testing.T) {
	root, err := fs.Sub(fixturesFS, "fixtures/routing")
	if err != nil {
		t.Fatal(err)
	}

	shm := core.NewSharedMemory()
	mux := http.NewServeMux()
	err = annotations.NewRoute(root, annotations.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.SetContext(core.SharedMemoryContext(rt.Context(), shm))
		rt.RegisterConstructor("SharedMemory", core.NewSharedMemoryBinding)
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

// TestRouteAPIEcho drives the /api/echo endpoint, which reads the request
// through HTTP\Request::current() and encodes the answer in PHP. It covers the
// three things the binding exists for: a request with no body, a trailing
// {rest...} path, and a JSON body that no superglobal carries.
func TestRouteAPIEcho(t *testing.T) {
	root, err := fs.Sub(fixturesFS, "fixtures/routing")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	if err := annotations.NewRoute(root).RegisterMux(mux); err != nil {
		t.Fatal(err)
	}

	echo := func(t *testing.T, req *http.Request) map[string]any {
		t.Helper()
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		var answer map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &answer); err != nil {
			t.Fatalf("decode %q: %v", rr.Body.String(), err)
		}
		return answer
	}

	// An empty object rather than an empty list: json_encode writes array() as
	// [], and the endpoint casts each collection to an object so a client
	// reading args.x finds an object to read it from.
	assertEmptyObject := func(t *testing.T, answer map[string]any, key string) {
		t.Helper()
		value, ok := answer[key].(map[string]any)
		if !ok || len(value) != 0 {
			t.Fatalf("%s = %#v, want an empty object", key, answer[key])
		}
	}

	t.Run("get with query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/echo?x=1&y=2", nil)
		req.Header.Set("X-Trace", "abc")
		answer := echo(t, req)

		if answer["method"] != http.MethodGet {
			t.Errorf("method = %v", answer["method"])
		}
		if answer["path"] != "/api/echo" {
			t.Errorf("path = %v", answer["path"])
		}
		if answer["rest"] != "" {
			t.Errorf("rest = %v, want empty", answer["rest"])
		}
		if answer["url"] != "http://example.com/api/echo?x=1&y=2" {
			t.Errorf("url = %v", answer["url"])
		}
		if answer["body"] != "" {
			t.Errorf("body = %v, want empty", answer["body"])
		}
		if answer["json"] != nil {
			t.Errorf("json = %v, want null", answer["json"])
		}
		args, _ := answer["args"].(map[string]any)
		if args["x"] != "1" || args["y"] != "2" {
			t.Errorf("args = %#v", answer["args"])
		}
		headers, _ := answer["headers"].(map[string]any)
		if headers["X-Trace"] != "abc" {
			t.Errorf("headers = %#v", answer["headers"])
		}
		assertEmptyObject(t, answer, "form")
	})

	// The trailing segments arrive joined, under the name the annotation wrote
	// rather than the one the router captured them as.
	t.Run("trailing path", func(t *testing.T) {
		answer := echo(t, httptest.NewRequest(http.MethodGet, "/api/echo/a/b/c", nil))

		if answer["rest"] != "a/b/c" {
			t.Errorf("rest = %v", answer["rest"])
		}
		if answer["path"] != "/api/echo/a/b/c" {
			t.Errorf("path = %v", answer["path"])
		}
		assertEmptyObject(t, answer, "args")
	})

	// A JSON body reaches the script through php://input, and leaves $_POST
	// empty: only the two form content types populate it, here as in PHP.
	t.Run("post json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/echo", strings.NewReader(`{"name":"ada","n":2}`))
		req.Header.Set("Content-Type", "application/json")
		answer := echo(t, req)

		if answer["method"] != http.MethodPost {
			t.Errorf("method = %v", answer["method"])
		}
		if answer["body"] != `{"name":"ada","n":2}` {
			t.Errorf("body = %v", answer["body"])
		}
		decoded, _ := answer["json"].(map[string]any)
		if decoded["name"] != "ada" || decoded["n"] != float64(2) {
			t.Errorf("json = %#v", answer["json"])
		}
		assertEmptyObject(t, answer, "form")
	})

	// A form body is the case the superglobals already answer: $_POST carries
	// it, and the raw bytes are still readable next to it.
	t.Run("post form", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/echo/x", strings.NewReader("title=q3"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		answer := echo(t, req)

		form, _ := answer["form"].(map[string]any)
		if form["title"] != "q3" {
			t.Errorf("form = %#v", answer["form"])
		}
		if answer["body"] != "title=q3" {
			t.Errorf("body = %v", answer["body"])
		}
		if answer["json"] != nil {
			t.Errorf("json = %v, want null", answer["json"])
		}
		if answer["rest"] != "x" {
			t.Errorf("rest = %v", answer["rest"])
		}
	})
}

func Example_routeSharedMemory() {
	root, err := fs.Sub(fixturesFS, "fixtures/routing")
	if err != nil {
		fmt.Println(err)
		return
	}

	shm := core.NewSharedMemory()
	mux := http.NewServeMux()
	err = annotations.NewRoute(root, annotations.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.SetContext(core.SharedMemoryContext(rt.Context(), shm))
		rt.RegisterConstructor("SharedMemory", core.NewSharedMemoryBinding)
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
