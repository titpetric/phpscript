package http_test

import (
	"context"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/http"
)

// TestRegister covers the four classes the binding installs. A script reaches
// them by name, so each one is constructed from PHP here, and the arguments
// with no useful default are refused through the same path: HTTP\Server carries
// the runtime the constructor closed over, so a refusal from Go reaches the
// script as a throw.
func TestRegister(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	http.Register(rt)
	rt.SetContext(context.Background())

	program, err := rt.Load(`<?php
$request = new HTTP\Request("post", "https://example.invalid/users");
$client = new HTTP\Client(array("timeout" => 5));
$mux = new HTTP\Mux();
$server = new HTTP\Server("127.0.0.1:0", $mux);

echo $request->method . " " . $request->url->path . " [" . $server->addr() . "]";

try {
	new HTTP\Server("", $mux);
	echo " no error";
} catch (Throwable $e) {
	echo " " . $e->getMessage();
}
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}

	want := `POST /users [] HTTP\Server: addr is required`
	if got := out.String(); got != want {
		t.Errorf("echoed %q, want %q", got, want)
	}
}

// TestClient_Post covers the one-line POST a script reaches for where PHP has
// curl_setopt. The body is optional, so a form POST with no payload is the same
// call with the argument left off, and an HTTP error status does not throw: the
// script reads $response->status().
func TestClient_Post(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/gone" {
			w.WriteHeader(nethttp.StatusNotFound)
		}
		_, _ = w.Write([]byte(r.Method + " " + r.URL.Path + " " + string(body)))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := http.NewClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	for _, test := range []struct {
		name   string
		path   string
		body   []string
		status int64
		want   string
	}{
		{name: "with a body", path: "/users", body: []string{`{"name":"ada"}`}, status: 200, want: `POST /users {"name":"ada"}`},
		{name: "without one", path: "/ping", status: 200, want: "POST /ping "},
		{name: "error status", path: "/gone", status: 404, want: "POST /gone "},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := client.Post(ctx, server.URL+test.path, test.body...)
			if err != nil {
				t.Fatalf("Post: %v", err)
			}
			if got := response.Status(); got != test.status {
				t.Errorf("Status() = %d, want %d", got, test.status)
			}
			if got := response.Body(); got != test.want {
				t.Errorf("Body() = %q, want %q", got, test.want)
			}
		})
	}
}

// newMux builds a router over a runtime that has declared the named PHP
// handlers, and returns it with the buffer the script and its handlers echo
// into. A handler has to be a PHP callable, so the functions are declared in a
// script and named by string.
func newMux(t *testing.T, declarations string) (*http.Mux, *strings.Builder) {
	t.Helper()

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	program, err := rt.Load(declarations)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}
	return http.NewMux(rt), &out
}

// TestNewMux covers the router a Go host builds directly. It starts with no
// routes, so a request before any handle() reaches net/http's own 404 and never
// a nil dereference.
func TestNewMux(t *testing.T) {
	t.Parallel()
	mux, _ := newMux(t, `<?php`)
	if mux == nil {
		t.Fatal("NewMux returned nil")
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(nethttp.MethodGet, "/nothing", nil))
	if recorder.Code != nethttp.StatusNotFound {
		t.Errorf("status = %d on a router with no routes, want %d", recorder.Code, nethttp.StatusNotFound)
	}
}

// TestMux_Handle covers describing a route. The handler is resolved where the
// route is written, so a name nothing declares is an error at handle() and not
// a 500 on the first request that arrives, and an empty pattern is refused
// outright.
func TestMux_Handle(t *testing.T) {
	t.Parallel()
	mux, _ := newMux(t, `<?php function ping($w, $r) { $w->write("pong"); }`)

	if err := mux.Handle("GET /ping", "ping"); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if err := mux.Handle("", "ping"); err == nil {
		t.Error("Handle accepted an empty pattern")
	} else if !strings.Contains(err.Error(), "pattern is required") {
		t.Errorf("error = %v, want one naming the pattern", err)
	}

	for _, handler := range []any{"undeclared", "", nil, int64(3)} {
		err := mux.Handle("GET /other", handler)
		if err == nil {
			t.Errorf("Handle accepted %#v as a handler", handler)
			continue
		}
		if !strings.Contains(err.Error(), "GET /other") {
			t.Errorf("error = %v, want one naming the route", err)
		}
	}
}

// TestMux_ServeHTTP covers answering one request, the path a Go host takes when
// it mounts a script's router without the script listening on anything. The two
// net/http values reach the handler, what it echoes becomes the response, and a
// throw is answered with a 500.
func TestMux_ServeHTTP(t *testing.T) {
	t.Parallel()
	mux, _ := newMux(t, `<?php
function hello($w, $r) {
	$w->header()->set("X-Answered-By", "php");
	$w->write("hello " . $r->path_value("name"));
}

function boom($w, $r) {
	throw new Exception("no");
}
`)
	if err := mux.Handle("GET /hello/{name}", "hello"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if err := mux.Handle("GET /boom", "boom"); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(nethttp.MethodGet, "/hello/ada", nil))
	if recorder.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
	if got := recorder.Body.String(); got != "hello ada" {
		t.Errorf("body = %q, want %q", got, "hello ada")
	}
	if got := recorder.Header().Get("X-Answered-By"); got != "php" {
		t.Errorf("X-Answered-By = %q, want php", got)
	}

	// A handler that threw before writing anything is answered with a 500.
	boom := httptest.NewRecorder()
	mux.ServeHTTP(boom, httptest.NewRequest(nethttp.MethodGet, "/boom", nil))
	if boom.Code != nethttp.StatusInternalServerError {
		t.Errorf("status = %d for a handler that threw, want %d", boom.Code, nethttp.StatusInternalServerError)
	}

	// A method the route does not name is net/http's own 405.
	wrong := httptest.NewRecorder()
	mux.ServeHTTP(wrong, httptest.NewRequest(nethttp.MethodPost, "/hello/ada", nil))
	if wrong.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d for POST on a GET route, want %d", wrong.Code, nethttp.StatusMethodNotAllowed)
	}
}
