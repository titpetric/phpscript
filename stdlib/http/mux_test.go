package http_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
//
// What it found out comes back through a binding rather than through echo: a
// handler's output goes to the response, and the point of this one is that
// there is no response left.
func TestMuxHandlerSeesItsOwnConnection(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	reported := make(chan string, 4)
	rt.RegisterFunc("report", func(what string) { reported <- what })

	program, err := rt.Load(`<?php
$mux = new HTTP\Mux();
$mux->handle("GET /slow", function ($w, $r) {
	// Without this the handler stops on the statement after the client goes.
	ignore_user_abort(true);
	$ticks = 0;
	while ($ticks < 200) {
		usleep(5000);
		$ticks++;
		if (connection_aborted()) {
			report("noticed and stopped");
			return;
		}
	}
	report("never noticed");
});
define("MUX", $mux);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	value, _ := rt.Const("MUX")
	handler := value.(http.Handler)

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

	// Waited for rather than read straight off: the handler asked to ignore the
	// disconnect, so it is still running after ServeHTTP has stopped waiting
	// for it. That is the whole point of ignore_user_abort, and it means the
	// report lands a moment after the request is over.
	select {
	case got := <-reported:
		if got != "noticed and stopped" {
			t.Errorf("handler reported %q, want it to have noticed", got)
		}
	case <-time.After(5 * time.Second):
		t.Error("the handler reported nothing")
	}
}

// TestMuxSkipsADeadConnection keeps the work off a request nobody is waiting
// for: the client was already gone, so the handler never ran.
func TestMuxSkipsADeadConnection(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	var ran atomic.Bool
	rt.RegisterFunc("mark_ran", func() { ran.Store(true) })

	program, err := rt.Load(`<?php
$mux = new HTTP\Mux();
$mux->handle("GET /work", function ($w, $r) { mark_ran(); $w->write("answered"); });
define("MUX", $mux);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	value, _ := rt.Const("MUX")
	handler := value.(http.Handler)

	gone, disconnect := context.WithCancel(context.Background())
	disconnect()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/work", nil).WithContext(gone))

	if ran.Load() {
		t.Error("the handler ran for a client that had already gone")
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

	addr := make(chan string, 1)
	rt.RegisterFunc("publish_addr", func(bound string) { addr <- bound })

	program, err := rt.Load(`<?php
$mux = new HTTP\Mux();
$mux->handle("GET /ping", function ($w, $r) { $w->write("pong"); });

$server = new HTTP\Server("127.0.0.1:0", $mux);
publish_addr($server->listen());
define("SERVER", $server);
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}

	bound := <-addr
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

// TestTwoMuxesShareOneRuntimeSafely is the defect the per-Mux mutex had: the
// lock was a field of Mux, so handlers on two muxes interpreted on one Runtime
// at the same time and took the process down with a concurrent map write.
func TestTwoMuxesShareOneRuntimeSafely(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	program, err := rt.Load(`<?php
$a = new HTTP\Mux();
$a->handle("GET /a", function ($w, $r) {
	$total = 0;
	for ($i = 0; $i < 60; $i++) { $total += strlen("abc") * $i; }
	$w->write("a:" . $total);
});

$b = new HTTP\Mux();
$b->handle("GET /b", function ($w, $r) {
	$parts = array();
	for ($i = 0; $i < 60; $i++) { $parts[] = strtoupper("x" . $i); }
	$w->write("b:" . count($parts));
});

define("A", $a);
define("B", $b);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	first, _ := rt.Const("A")
	second, _ := rt.Const("B")
	muxes := []struct {
		handler http.Handler
		path    string
		want    string
	}{
		{first.(http.Handler), "/a", "a:5310"},
		{second.(http.Handler), "/b", "b:60"},
	}

	var wg sync.WaitGroup
	var bad atomic.Int64
	for _, mux := range muxes {
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 25 {
					response := httptest.NewRecorder()
					mux.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, mux.path, nil))
					if response.Body.String() != mux.want {
						bad.Add(1)
					}
				}
			}()
		}
	}
	wg.Wait()

	if got := bad.Load(); got != 0 {
		t.Errorf("wrong answers = %d, want 0", got)
	}
}

// TestScriptDoesNotRaceItsOwnHandlers is the other half: listen() returns and
// the script keeps interpreting, so the script's goroutine and net/http's were
// both running PHP on one runtime. wait() parks the runtime; everything before
// it holds it.
func TestScriptDoesNotRaceItsOwnHandlers(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	// The address comes out through a binding rather than a constant the test
	// reads back: rt.Const is runtime state, and reading it from another
	// goroutine while Run is writing is the very thing this test is about.
	addr := make(chan string, 1)
	rt.RegisterFunc("publish_addr", func(bound string) { addr <- bound })

	program, err := rt.Load(`<?php
$mux = new HTTP\Mux();
$mux->handle("GET /ping", function ($w, $r) {
	$total = 0;
	for ($i = 0; $i < 40; $i++) { $total += $i; }
	$w->write("pong:" . $total);
});

$server = new HTTP\Server("127.0.0.1:0", $mux);
$bound = $server->listen();
publish_addr($bound);

// Work between listen() and wait(), which is the window the script's own
// goroutine used to interpret in while handlers were answering.
$noise = 0;
for ($i = 0; $i < 4000; $i++) { $noise += strlen("abcdef") + $i; }

publish_noise($noise);
define("SERVER", $server);
`)
	if err != nil {
		t.Fatal(err)
	}

	noise := make(chan int64, 1)
	rt.RegisterFunc("publish_noise", func(total int64) { noise <- total })

	// The requests start while Run is still executing the loop above.
	var wg sync.WaitGroup
	var bad atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		bound := <-addr
		var inner sync.WaitGroup
		for range 6 {
			inner.Add(1)
			go func() {
				defer inner.Done()
				for range 10 {
					response, err := http.Get("http://" + bound + "/ping")
					if err != nil {
						continue
					}
					body, _ := io.ReadAll(response.Body)
					_ = response.Body.Close()
					if string(body) != "pong:780" {
						bad.Add(1)
					}
				}
			}()
		}
		inner.Wait()
	}()

	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	wg.Wait()

	if got := <-noise; got != 8022000 {
		t.Errorf("the script's own arithmetic came out %d, want 8022000", got)
	}
	if got := bad.Load(); got != 0 {
		t.Errorf("wrong handler answers = %d, want 0", got)
	}

	stop, _ := rt.Load(`<?php $s = SERVER; $s->shutdown();`)
	_ = rt.Run(stop)
}

// TestMuxTakesAHandlerFromAProperty is a server class holding its handlers in
// properties and registering each as $this->fnName. The closure binds $this,
// which comes along and is shared by every request, so a handler reads its
// configuration and writes to its arguments.
func TestMuxTakesAHandlerFromAProperty(t *testing.T) {
	_, handler := serveScript(t, `<?php
class Site {
	public $greeting;
	public $hello;

	public function boot() {
		$this->greeting = "hei";
		$this->hello = function ($w, $r) {
			$w->write($this->greeting . " " . $r->url->query()->get("name"));
		};
	}
}

$site = new Site;
$site->boot();

$mux = new HTTP\Mux();
$mux->handle("GET /hello", $site->hello);
define("MUX", $mux);
`)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello?name=tit", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if got := response.Body.String(); got != "hei tit" {
		t.Errorf("body = %q, want %q", got, "hei tit")
	}
}

// TestMuxTakesABoundMethod is the shape testdata/testserver.php uses: the
// handlers are methods and mount() registers each as $this->fnName, with no
// property and no constructor in between. The receiver comes along the way a
// closure's captured $this does, and it is the same object every request is
// answered against.
func TestMuxTakesABoundMethod(t *testing.T) {
	_, handler := serveScript(t, `<?php
class Site {
	public function mount($mux) {
		$mux->handle("GET /hello", $this->hello);
		$mux->handle("GET /users/{id}", $this->showUser);
	}

	public function hello($w, $r) {
		$w->write("hei " . $r->url->query()->get("name"));
	}

	public function showUser($w, $r) {
		$w->write("user " . $r->path_value("id"));
	}
}

$mux = new HTTP\Mux();
(new Site)->mount($mux);
define("MUX", $mux);
`)

	for _, want := range []struct{ path, body string }{
		{"/hello?name=tit", "hei tit"},
		{"/users/42", "user 42"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, want.path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, body = %q", want.path, response.Code, response.Body.String())
		}
		if got := response.Body.String(); got != want.body {
			t.Errorf("GET %s: body = %q, want %q", want.path, got, want.body)
		}
	}
}

// TestMuxTakesAStaticMethodAndAnInvokable is the rest of what AsCallable takes:
// "Class::method", which has no receiver to share, and an object declaring
// __invoke, which is a bound method under the name php reserves for one.
func TestMuxTakesAStaticMethodAndAnInvokable(t *testing.T) {
	_, handler := serveScript(t, `<?php
class Site {
	static function hello($w, $r) {
		$w->write("hei " . $r->url->query()->get("name"));
	}
}

class Greeter {
	public $greeting = "moi";

	public function __invoke($w, $r) {
		$w->write($this->greeting . " " . $r->url->query()->get("name"));
	}
}

$mux = new HTTP\Mux();
$mux->handle("GET /hello", "Site::hello");
$mux->handle("GET /greet", new Greeter);
define("MUX", $mux);
`)

	for _, want := range []struct{ path, body string }{
		{"/hello?name=tit", "hei tit"},
		{"/greet?name=tit", "moi tit"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, want.path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, body = %q", want.path, response.Code, response.Body.String())
		}
		if got := response.Body.String(); got != want.body {
			t.Errorf("GET %s: body = %q, want %q", want.path, got, want.body)
		}
	}
}

// TestMuxRefusesAnArrayCallable pins the one callable spelling the router does
// not take, and the words it is refused in. It is a callable everywhere else; a
// handler is a closure or a method read off its receiver, and
// array($object, "method") is neither. A refusal that named neither the
// spelling nor the alternative was the whole of what a script used to be told.
func TestMuxRefusesAnArrayCallable(t *testing.T) {
	tests := []struct {
		name    string
		handler string
		want    string
	}{
		{
			name:    "array with an object",
			handler: `array($site, "hello")`,
			want:    `the array($object, "method") spelling is not a handler here`,
		},
		{
			name:    "array with a class name",
			handler: `array("Site", "hello")`,
			want:    `the array($object, "method") spelling is not a handler here`,
		},
		{
			name:    "an int is no spelling at all",
			handler: `42`,
			want:    "a callable is a closure, an object with __invoke",
		},
		{
			name:    "a class that declares no __invoke",
			handler: `new Site`,
			want:    "a callable is a closure, an object with __invoke",
		},
		{
			name:    "a method the class does not declare",
			handler: `"Site::missing"`,
			want:    "the class declares no method of that name",
		},
		{
			name:    "a class that is not declared",
			handler: `"Missing::hello"`,
			want:    "no class of that name is declared",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			rt := runner.New(&out, runner.Options{})
			stdlib.Register(rt)

			program, err := rt.Load(`<?php
class Site {
	public function hello($w, $r) { $w->write("hi"); }
}
$site = new Site;
$mux = new HTTP\Mux();
$mux->handle("GET /hello", ` + test.handler + `);
`)
			if err != nil {
				t.Fatal(err)
			}
			if err = rt.Run(program); err == nil {
				t.Fatalf("%s was accepted as a handler", test.handler)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error %q does not contain %q", err, test.want)
			}
		})
	}
}

// TestMuxQueuesBeyondItsWorkers holds the shape the pool is: workers are the
// parallelism and the queue is what waits behind them. Two workers against six
// slow requests is three rounds, not six and not one.
func TestMuxQueuesBeyondItsWorkers(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	program, err := rt.Load(`<?php
$mux = new HTTP\Mux();
$mux->handle("GET /slow", function ($w, $r) {
	usleep(150000);
	$w->write("done");
});
define("MUX", $mux);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	value, _ := rt.Const("MUX")
	handler := value.(http.Handler)

	// Two workers, so six requests of 150ms are three rounds: over 300ms and
	// well under the 900ms they would take one at a time.
	if mux, ok := value.(interface{ UsePoolForTest(*runner.Pool) }); ok {
		mux.UsePoolForTest(runner.NewPool(rt, 2, 64, nil))
	}

	const requests = 6
	started := time.Now()

	var wg sync.WaitGroup
	var bad atomic.Int64
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/slow", nil))
			if response.Body.String() != "done" {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := bad.Load(); got != 0 {
		t.Fatalf("failed requests = %d", got)
	}
	elapsed := time.Since(started)
	if elapsed > 800*time.Millisecond {
		t.Errorf("%d requests took %s, which is one at a time", requests, elapsed)
	}
}

// A handler answers on a runtime of its own, so every call it makes has to run
// on that runtime. A callable the script built before the server started is a
// value like any other - it crosses into a worker as a capture, or is read off a
// shared object - and calling one there must not reach back into the runtime
// that built it: that runtime is unguarded by design (see runner/fork.go) and
// the response it would write to belongs to a different request.
//
// Each case below is one spelling of that call, and every callable in them
// echoes. An echo is what proves which runtime ran the body: the response writer
// is pushed onto the worker only, so an echo that reaches the response ran
// there, and one that reaches the process output ran on the parent.
var reentrySpellings = []struct {
	name  string
	route string
}{
	{"captured callable", `$mux->handle("GET /x", wrap($site->index));`},
	{"captured closure", `$mux->handle("GET /x", function ($w, $r) use ($echo) { $echo($w, $r); });`},
	{"nested captures", `$mux->handle("GET /x", wrap(wrap($site->index)));`},
	{"property holding a closure", `$mux->handle("GET /x", function ($w, $r) use ($site) { ($site->boxed)($w, $r); });`},
	{"array element holding a closure", `$mux->handle("GET /x", function ($w, $r) use ($site) { ($site->bag["one"])($w, $r); });`},
	{"bound method off a shared object", `$mux->handle("GET /x", $site->index);`},
	{"call_user_func", `$mux->handle("GET /x", function ($w, $r) use ($echo) { call_user_func($echo, $w, $r); });`},
	{"call_user_func_array", `$mux->handle("GET /x", function ($w, $r) use ($echo) { call_user_func_array($echo, array($w, $r)); });`},
	{"usort comparator", `$mux->handle("GET /x", function ($w, $r) use ($cmp) { $a = array(2, 1); usort($a, $cmp); });`},
	{"array_map", `$mux->handle("GET /x", function ($w, $r) use ($site) { array_map($site->twice, array(1)); });`},
	{"array_filter", `$mux->handle("GET /x", function ($w, $r) use ($keep) { array_filter(array(1), $keep); });`},
	{"array_reduce", `$mux->handle("GET /x", function ($w, $r) use ($add) { array_reduce(array(1), $add, 0); });`},
	{"preg_replace_callback", `$mux->handle("GET /x", function ($w, $r) use ($site) { preg_replace_callback('/\d/', $site->digit, "a1"); });`},
	{"shutdown callback", `$mux->handle("GET /x", function ($w, $r) use ($echo) { register_shutdown_function($echo); $echo($w, $r); });`},
	{"autoloader", `$mux->handle("GET /x", function ($w, $r) { class_exists("Missing"); });`},
}

// reentryScript builds one script per spelling: the same shared state, built on
// the runtime that runs the script, and one route.
func reentryScript(route string) string {
	return `<?php
class Site {
	public $boxed;
	public $twice;
	public $digit;
	public $bag;
	function boot() {
		$this->boxed = function ($w, $r) { echo "inner\n"; };
		$this->twice = function ($n) { echo "inner\n"; return $n * 2; };
		$this->digit = function ($m) { echo "inner\n"; return "<" . $m[0] . ">"; };
		$this->bag = array("one" => function ($w, $r) { echo "inner\n"; });
	}
	function index($w, $r) { echo "inner\n"; }
}
function wrap($next) {
	return function ($w, $r) use ($next) { $next($w, $r); };
}
$site = new Site;
$site->boot();
$echo = function ($w, $r) { echo "inner\n"; };
// An autoloader is script state a fork carries, because it is how a name becomes
// a declaration. A worker that is the first to name a class has to be able to
// load it, and the loading has to happen on the worker.
spl_autoload_register(function ($class) { echo "inner\n"; });
$cmp = function ($a, $b) { echo "inner\n"; return $a - $b; };
$keep = function ($n) { echo "inner\n"; return true; };
$add = function ($carry, $n) { echo "inner\n"; return $carry + $n; };
$mux = new HTTP\Mux();
` + route + `
define("MUX", $mux);
`
}

// TestReentryAnswersOnTheWorker holds that the call reached the worker: what it
// echoed is in the response, and the process output the parent runtime writes to
// is untouched.
func TestReentryAnswersOnTheWorker(t *testing.T) {
	for _, spelling := range reentrySpellings {
		t.Run(spelling.name, func(t *testing.T) {
			out, handler := serveScriptOutput(t, reentryScript(spelling.route))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/x", nil))

			if got := response.Body.String(); !strings.Contains(got, "inner") {
				t.Errorf("response = %q, want the echo of the callable the handler called", got)
			}
			if got := out.String(); got != "" {
				t.Errorf("the parent runtime wrote %q; the call ran on the runtime that built the value", got)
			}
		})
	}
}

// TestReentryIsNotARace drives each spelling concurrently. A call that reached
// the parent runtime is two goroutines inside one unguarded execution, which the
// race detector reports rather than the test asserting it.
func TestReentryIsNotARace(t *testing.T) {
	for _, spelling := range reentrySpellings {
		t.Run(spelling.name, func(t *testing.T) {
			_, handler := serveScriptOutput(t, reentryScript(spelling.route))
			var wg sync.WaitGroup
			for i := range 16 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
						fmt.Sprintf("/x?n=%d", i), nil))
				}()
			}
			wg.Wait()
		})
	}
}
