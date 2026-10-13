package http_test

import (
	"context"
	"io"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/http"
)

// newServer builds a server over a handler answering "pong" on every path,
// together with the runtime it belongs to. The handler is a plain net/http one
// and no HTTP\Mux, so the listening half is read on its own: a Mux brings a
// worker pool with it, and none of these tests is about that.
func newServer(t *testing.T, ctx context.Context) (*runner.Runtime, *http.Server) {
	t.Helper()

	rt := runner.New(io.Discard, runner.Options{})
	rt.SetContext(ctx)

	server, err := http.NewServer(rt, "127.0.0.1:0", nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_, _ = w.Write([]byte("pong"))
	}))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return rt, server
}

// get reads one response body over the loopback address.
func get(t *testing.T, addr string) string {
	t.Helper()

	response, err := nethttp.Get("http://" + addr + "/ping")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// TestServer covers construction. Building a server binds nothing, so the two
// arguments with no useful default are checked here and the address stays empty
// until listen() runs: a script that built a server has not taken a port yet.
func TestServer(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	rt.SetContext(context.Background())
	handler := nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) {})

	if _, err := http.NewServer(rt, "", handler); err == nil {
		t.Error("NewServer accepted an empty addr")
	} else if !strings.Contains(err.Error(), "addr is required") {
		t.Errorf("error = %v, want one naming addr", err)
	}

	// A handler that answers no request is refused where the server is built,
	// ahead of a nil dereference on the first request.
	if _, err := http.NewServer(rt, "127.0.0.1:0", "not a handler"); err == nil {
		t.Error("NewServer accepted a handler that answers no requests")
	} else if !strings.Contains(err.Error(), "must answer requests") {
		t.Errorf("error = %v, want one naming the handler", err)
	}

	server, err := http.NewServer(rt, "127.0.0.1:0", handler, 4, 128)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if got := server.Addr(); got != "" {
		t.Errorf("Addr() = %q on a server that has not listened, want empty", got)
	}
}

// TestServer_Listen covers binding the port. It returns the address the system
// picked, so a script that bound port 0 can report where it is, and it does
// not block: the script carries on and calls wait() when it has nothing left.
//
// A second listen on a running server is an error, because the first address is
// still being served and a silent rebind would lose it.
func TestServer_Listen(t *testing.T) {
	_, server := newServer(t, context.Background())
	t.Cleanup(server.Shutdown)

	bound, err := server.Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if bound == "" {
		t.Fatal("Listen returned no address")
	}
	if got := get(t, bound); got != "pong" {
		t.Errorf("body = %q, want pong", got)
	}

	again, err := server.Listen()
	if err == nil {
		t.Fatalf("a second Listen returned %q, want an error", again)
	}
	if !strings.Contains(err.Error(), "already listening") {
		t.Errorf("error = %v, want one naming the address already bound", err)
	}
	if got := server.Addr(); got != bound {
		t.Errorf("Addr() = %q after the refused Listen, want the first address %q", got, bound)
	}
}

// TestServer_Addr covers the address a script reads back. It is empty before
// listen() and empty again once the server has stopped, so the field answers
// where requests are being taken and never where they once were.
func TestServer_Addr(t *testing.T) {
	_, server := newServer(t, context.Background())

	if got := server.Addr(); got != "" {
		t.Errorf("Addr() before Listen = %q, want empty", got)
	}

	bound, err := server.Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if got := server.Addr(); got != bound {
		t.Errorf("Addr() = %q, want the bound address %q", got, bound)
	}

	server.Shutdown()
	if got := server.Addr(); got != "" {
		t.Errorf("Addr() after Shutdown = %q, want empty", got)
	}
}

// TestServer_Shutdown covers the stop a register_shutdown_function callback
// makes. The callback runs when the script has already ended and has nowhere to
// put a failure, so shutdown answers nothing and throws nothing: calling it
// twice, or on a server that never listened, is no error.
func TestServer_Shutdown(t *testing.T) {
	_, server := newServer(t, context.Background())

	// A server that never listened has nothing to stop.
	server.Shutdown()

	bound, err := server.Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if got := get(t, bound); got != "pong" {
		t.Errorf("body = %q, want pong", got)
	}

	server.Shutdown()
	// A second one is the same no-op as the first.
	server.Shutdown()

	if _, err := nethttp.Get("http://" + bound + "/ping"); err == nil {
		t.Error("the server was still answering after Shutdown")
	}

	// The port is free again, so the address can be taken by a new server.
	rt := runner.New(io.Discard, runner.Options{})
	rt.SetContext(context.Background())
	replacement, err := http.NewServer(rt, bound, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {}))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if _, err := replacement.Listen(); err != nil {
		t.Errorf("Listen on the released address: %v", err)
	}
	replacement.Shutdown()
}

// TestServer_Close covers the stop that drops what is in flight. A script with
// answers nothing is waiting for calls it, so it reports a failure where
// shutdown swallows one, and on a server that never listened it reports none.
func TestServer_Close(t *testing.T) {
	_, server := newServer(t, context.Background())

	if err := server.Close(); err != nil {
		t.Errorf("Close on a server that never listened = %v, want nil", err)
	}

	bound, err := server.Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if got := get(t, bound); got != "pong" {
		t.Errorf("body = %q, want pong", got)
	}

	if err := server.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := nethttp.Get("http://" + bound + "/ping"); err == nil {
		t.Error("the server was still answering after Close")
	}
	if got := server.Addr(); got != "" {
		t.Errorf("Addr() after Close = %q, want empty", got)
	}
	// Close and Shutdown each stop the server once, however often they are
	// called and whichever arrives first.
	if err := server.Close(); err != nil {
		t.Errorf("a second Close = %v, want nil", err)
	}
	server.Shutdown()
}

// TestServer_Wait covers what a serving script blocks on. The clock belongs to
// the runtime, so wait() returns when the script runs out of time or the client
// that started it goes away, and never on a signal of the server's own.
func TestServer_Wait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, server := newServer(t, ctx)
	t.Cleanup(server.Shutdown)

	if _, err := server.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}

	returned := make(chan struct{})
	go func() {
		server.Wait()
		close(returned)
	}()

	// Still serving: the runtime's context is live, so wait() holds.
	select {
	case <-returned:
		t.Fatal("Wait returned while the runtime context was live")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after the runtime context ended")
	}
}
