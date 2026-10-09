package tests_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	phphttp "github.com/titpetric/phpscript/stdlib/http"
)

// This file prices one request on Path B, which is HTTP\Mux on runner.Pool: the
// program is parsed once, each handler is resolved once at $mux->handle(), the
// workers are forked once, and the response is streamed. `phpscript server` is
// Path A and builds a runtime per request; a number from one never belongs in a
// table with a number from the other. docs/agents/performance.md owns the
// comparison and scripts/bench-http.sh is the latency half of the same harness.
//
// It is the real testdata/testserver.php, and no copy of it. The file's class
// declaration is taken and its two trailing statements are left, because
// run() calls listen() and then parks in wait() until the script's time limit
// ends it, which a benchmark cannot do. The driver below is what run() does up
// to the point of binding a socket.
//
// The socket is outside the measurement: the writer discards, with no pass
// through net/http's connection buffers. That cost is the same on the Go twin,
// so it belongs to the sweep.

// benchServerDriver stands in for run(): the limit the real script sets before
// it listens, then the router passed to Go in place of HTTP\Server. The limit
// is the script's and the forks do not inherit it, there as here; what arms a
// worker's per-statement deadline check is the request's own context, which is
// why the request below carries a cancellable one.
const benchServerDriver = `<?php
set_time_limit(3600);
bench_mux((new Server)->mount());
`

// benchServerRoutes are the four routes that price the request path. /slow is
// twenty usleep calls and prices the deadline machinery instead; /info is
// phpinfo(). docs/agents/performance.md says why each is in or out.
var benchServerRoutes = []struct {
	name   string
	method string
	target string
	body   string
	// want is a distinctive substring of the response, checked once before the
	// timed loop. It is what makes the benchmark notice testserver.php changing
	// under it, and never quietly measures a 404.
	want string
}{
	{name: "hello", method: http.MethodGet, target: "/hello?name=sprint", want: "hello sprint"},
	{name: "users", method: http.MethodGet, target: "/users/42", want: `"id":"42"`},
	{name: "echo", method: http.MethodPost, target: "/echo", body: "name=sprint", want: `"name":"sprint"`},
	{name: "index", method: http.MethodGet, target: "/", want: "GET  /hello        a greeting"},
}

// BenchmarkTestServerRoute answers one request per iteration through the router
// testdata/testserver.php builds, on the pool HTTP\Server::listen would install.
func BenchmarkTestServerRoute(b *testing.B) {
	handler := newTestServerHandler(b, 1, 64)

	for _, route := range benchServerRoutes {
		b.Run(route.name, func(b *testing.B) {
			request, rewind := benchServerRequest(b, route.method, route.target, route.body)

			check := httptest.NewRecorder()
			rewind()
			handler.ServeHTTP(check, request)
			if got := check.Body.String(); !strings.Contains(got, route.want) {
				b.Fatalf("%s %s: body %q does not contain %q", route.method, route.target, got, route.want)
			}

			writer := &benchServerWriter{header: make(http.Header, 4)}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				rewind()
				handler.ServeHTTP(writer, request)
			}
			b.StopTimer()
			if writer.written == 0 {
				b.Fatal("handler wrote nothing")
			}
		})
	}
}

// newTestServerHandler declares testdata/testserver.php's class, mounts its
// router the way run() does, and installs the worker pool listen() would.
func newTestServerHandler(b *testing.B, workers, queue int) http.Handler {
	b.Helper()

	source, err := os.ReadFile("../testdata/testserver.php")
	if err != nil {
		b.Fatal(err)
	}
	program, err := parser.Parse(string(source))
	if err != nil {
		b.Fatal(err)
	}
	driver, err := parser.Parse(benchServerDriver)
	if err != nil {
		b.Fatal(err)
	}

	// The declarations, and nothing the file runs at top level. A statement that
	// is not a class declaration is `$server = new Server;` and `$server->run();`,
	// and the second of those never returns.
	mounted := &model.Program{Namespace: program.Namespace}
	for _, stmt := range program.Stmts {
		if _, ok := stmt.(*model.ClassDecl); ok {
			mounted.Stmts = append(mounted.Stmts, stmt)
		}
	}
	if len(mounted.Stmts) == 0 {
		b.Fatal("testdata/testserver.php declares no class")
	}
	mounted.Stmts = append(mounted.Stmts, driver.Stmts...)

	var routed http.Handler
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	rt.RegisterFunc("bench_mux", func(value any) error {
		mux, ok := value.(http.Handler)
		if !ok {
			b.Errorf("mount() answered %T, which does not answer requests", value)
			return nil
		}
		routed = mux
		return nil
	})

	if err := rt.Run(mounted); err != nil {
		b.Fatal(err)
	}
	if routed == nil {
		b.Fatal("mount() handed back no router")
	}

	// The pool listen() installs, sized here and not from GOMAXPROCS: the
	// pinned benchmark job runs under taskset and a default pool would be one
	// worker there and four elsewhere. A worker count does not change what one
	// request allocates, and holding it still is what makes two runs comparable.
	mux, ok := routed.(*phphttp.Mux)
	if !ok {
		b.Fatalf("mount() answered %T, not an HTTP\\Mux", routed)
	}
	mux.UsePoolForTest(runner.NewPool(rt, workers, queue, nil))
	return mux
}

// benchServerRequest builds one request and the function that puts it back the
// way it arrived, so the loop reuses it and allocates none per iteration.
//
// The context is cancellable, where httptest's is a background one. A served
// request's context can end, which arms the runtime's per-statement deadline
// check and is what the mux reads before passing the request to a worker;
// a context with no Done channel would price an interpreter that checks nothing.
//
// A body has to be rewound and the parsed form dropped: form_value() parses on
// first call and net/http caches the result on the request, so a reused request
// would answer the second iteration out of the first one's cache and price
// nothing.
func benchServerRequest(b *testing.B, method, target, body string) (*http.Request, func()) {
	b.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	if body == "" {
		return httptest.NewRequest(method, target, nil).WithContext(ctx), func() {}
	}
	reader := &benchServerBody{Reader: strings.NewReader(body)}
	request := httptest.NewRequest(method, target, reader.Reader).WithContext(ctx)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Body = reader
	return request, func() {
		_, _ = reader.Seek(0, io.SeekStart)
		request.Form, request.PostForm = nil, nil
	}
}

// benchServerBody is a request body that can be read again, built once outside
// the timed loop.
type benchServerBody struct {
	*strings.Reader
}

func (b *benchServerBody) Close() error { return nil }

// benchServerWriter discards the response and records that there was one. A
// httptest.ResponseRecorder would grow a buffer per iteration and put its own
// allocations in the number.
type benchServerWriter struct {
	header  http.Header
	written int
}

func (w *benchServerWriter) Header() http.Header { return w.header }

func (w *benchServerWriter) WriteHeader(int) {}

func (w *benchServerWriter) Write(p []byte) (int, error) {
	w.written += len(p)
	return len(p), nil
}
