package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// serveScript runs src, which is expected to build a router and leave it in
// $mux, and returns the runtime and the handler a Go test can drive.
//
// Nothing listens: a mux answers a request, so the routing half is testable
// without a port, and only the listening tests take one.
func serveScript(t *testing.T, src string) (*runner.Runtime, http.Handler) {
	t.Helper()
	_, rt, handler := serveScriptAll(t, src)
	return rt, handler
}

// serveScriptOutput is serveScript for a test that reads what a handler echoed,
// which is where a handler answering nobody puts what it found out.
func serveScriptOutput(t *testing.T, src string) (*strings.Builder, http.Handler) {
	t.Helper()
	out, _, handler := serveScriptAll(t, src)
	return out, handler
}

func serveScriptAll(t *testing.T, src string) (*strings.Builder, *runner.Runtime, http.Handler) {
	t.Helper()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	program, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}

	value, ok := rt.Const("MUX")
	if !ok {
		t.Fatal("the script defined no MUX constant")
	}
	handler, ok := value.(http.Handler)
	if !ok {
		t.Fatalf("MUX is %T, want an http.Handler", value)
	}
	return &out, rt, handler
}

// TestMuxRoutesToPHP holds the whole of the routing contract: net/http's
// patterns, the two net/http values handed to the handler, and the path
// segments read back off the request.
func TestMuxRoutesToPHP(t *testing.T) {
	_, handler := serveScript(t, `<?php
$mux = new HTTP\Mux();

$mux->handle("GET /hello", function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("X-Answered-By", "php");
	$w->write("hello " . $r->url->query()->get("name"));
});

$mux->handle("GET /users/{id}", function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	(new JSON\Encoder($w))->encode(array("id" => $r->path_value("id")));
});

$mux->handle("POST /echo", function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->write($r->method . ":" . $r->form_value("name"));
});

define("MUX", $mux);
`)

	tests := []struct {
		name    string
		request *http.Request
		want    string
	}{
		{
			name:    "query",
			request: httptest.NewRequest(http.MethodGet, "/hello?name=tit", nil),
			want:    "hello tit",
		},
		{
			name:    "path value",
			request: httptest.NewRequest(http.MethodGet, "/users/42", nil),
			want:    "{\"id\":\"42\"}\n",
		},
		{
			name:    "form body",
			request: formRequest("/echo", "name=alice"),
			want:    "POST:alice",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, test.request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if got := response.Body.String(); got != test.want {
				t.Errorf("body = %q, want %q", got, test.want)
			}
		})
	}
}

// TestMuxUnroutedIsNotFound leaves the router's own answers to the router: a
// pattern nothing matched is net/http's 404, not a PHP error.
func TestMuxUnroutedIsNotFound(t *testing.T) {
	_, handler := serveScript(t, `<?php
$mux = new HTTP\Mux();
$mux->handle("GET /hello", function ($w, $r) { $w->write("hi"); });
define("MUX", $mux);
`)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/nothing", nil))
	if response.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

// TestMuxHandlerThrowIsOneRequestsProblem keeps a failing handler from taking
// the server with it: the request gets a status and the next one is answered.
func TestMuxHandlerThrowIsOneRequestsProblem(t *testing.T) {
	_, handler := serveScript(t, `<?php
$mux = new HTTP\Mux();
$mux->handle("GET /boom", function ($w, $r) { throw new Exception("detonated"); });
$mux->handle("GET /fine", function ($w, $r) { $w->write("still here"); });
define("MUX", $mux);
`)

	// No Runtime.OnError here on purpose. Installing one means "report the
	// error and carry on from the next statement" for every PHP error, so the
	// throw would never reach the handler wrapper and the request would be
	// answered 200 with an empty body. That is the runtime's contract rather
	// than this binding's, and the doc on Handle says so.
	boom := httptest.NewRecorder()
	handler.ServeHTTP(boom, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if boom.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", boom.Code, http.StatusInternalServerError)
	}

	fine := httptest.NewRecorder()
	handler.ServeHTTP(fine, httptest.NewRequest(http.MethodGet, "/fine", nil))
	if got := fine.Body.String(); got != "still here" {
		t.Errorf("the next request answered %q, want %q", got, "still here")
	}
}

// TestMuxHandlerSeesItsOwnConnection is what EnterRequest is for: a client
// that leaves mid-request is the one the handler asks about, and the handler
// gets to decide what to do about it.
func TestMuxHandlerSeesItsOwnConnection(t *testing.T) {
	out, handler := serveScriptOutput(t, `<?php
$mux = new HTTP\Mux();
$mux->handle("GET /slow", function ($w, $r) {
	// Without this the handler stops on the statement after the client goes.
	ignore_user_abort(true);
	$ticks = 0;
	while ($ticks < 200) {
		usleep(5000);
		$ticks++;
		if (connection_aborted()) {
			echo "noticed and stopped";
			return;
		}
	}
	echo "never noticed";
});
define("MUX", $mux);
`)

	leaving, disconnect := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/slow", nil).WithContext(leaving)

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}()

	time.Sleep(50 * time.Millisecond)
	disconnect()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not notice the client leaving")
	}

	if got := out.String(); !strings.Contains(got, "noticed and stopped") {
		t.Errorf("handler wrote %q, want it to have noticed", got)
	}
}

// TestMuxSkipsADeadConnection keeps the work off a request nobody is waiting
// for: the client was already gone, so the handler never ran.
func TestMuxSkipsADeadConnection(t *testing.T) {
	out, handler := serveScriptOutput(t, `<?php
$mux = new HTTP\Mux();
$mux->handle("GET /work", function ($w, $r) { echo "ran"; $w->write("answered"); });
define("MUX", $mux);
`)

	gone, disconnect := context.WithCancel(context.Background())
	disconnect()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/work", nil).WithContext(gone))

	if got := out.String(); got != "" {
		t.Errorf("the handler ran and wrote %q, want it skipped", got)
	}
	if got := response.Body.String(); got != "" {
		t.Errorf("body = %q, want nothing written", got)
	}
}

// TestServerListensAndStops drives the listening half: a port the system
// picked, a request answered over it, and a shutdown that closes it.
func TestServerListensAndStops(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	program, err := rt.Load(`<?php
$mux = new HTTP\Mux();
$mux->handle("GET /ping", function ($w, $r) { $w->write("pong"); });

$server = new HTTP\Server("127.0.0.1:0", $mux);
$bound = $server->listen();

define("ADDR", $bound);
define("SERVER", $server);
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}

	addr, _ := rt.Const("ADDR")
	bound, _ := addr.(string)
	if bound == "" {
		t.Fatal("listen answered no address")
	}

	response, err := http.Get("http://" + bound + "/ping")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(body) != "pong" {
		t.Errorf("body = %q, want %q", body, "pong")
	}

	// Stopping it is the script's to do, from wherever it decides to: here a
	// second program on the same runtime, as a shutdown callback would.
	stop, err := rt.Load(`<?php $server = SERVER; $server->shutdown();`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(stop); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := http.Get("http://" + bound + "/ping"); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the server was still answering after its shutdown callback ran")
}

// formRequest builds a urlencoded POST, which is the shape form_value reads.
func formRequest(target, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}
