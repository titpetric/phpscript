//go:build ignore

// testserver.go is testserver.php written in Go, route for route, so the two
// can be put through the same load generator and the difference read off.
//
// It is the same work: the same patterns on the same net/http router, the same
// JSON objects encoded with the same encoder, the same reply bodies. What it
// does not have is a VM, which is the whole of what the comparison measures.
//
// TESTSERVER_WORKERS bounds how many requests are answered at once, the way the
// PHP server's worker count does. Go does not need it - net/http answers each
// request on a goroutine and the scheduler sorts it out - so unset is unbounded
// and is the Go server anyone would actually write. Set, it is a semaphore of
// that depth, which is what makes a side by side against a bounded PHP server
// a comparison of the same arrangement rather than of two different ones.
//
//	go run testdata/testserver.go
//	TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS=5 go run testdata/testserver.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type config struct {
	addr    string
	workers int
	limit   time.Duration
}

func readConfig() config {
	addr := os.Getenv("TESTSERVER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8099"
	}
	workers, _ := strconv.Atoi(os.Getenv("TESTSERVER_WORKERS"))
	limit, _ := strconv.Atoi(os.Getenv("TESTSERVER_LIMIT"))
	if limit < 1 {
		limit = 10
	}
	return config{addr: addr, workers: workers, limit: time.Duration(limit) * time.Second}
}

func routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", index)
	mux.HandleFunc("GET /hello", hello)
	mux.HandleFunc("GET /users/{id}", showUser)
	mux.HandleFunc("POST /echo", echoRequest)
	mux.HandleFunc("GET /slow", slow)
	mux.HandleFunc("GET /info", info)
	return mux
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "phpscript test server\n\n")
	fmt.Fprint(w, "GET  /             this page ('{$}' anchors it; a bare\n")
	fmt.Fprint(w, "                   'GET /' is a net/http subtree and would\n")
	fmt.Fprint(w, "                   answer for every path nothing else claimed)\n")
	fmt.Fprint(w, "GET  /hello        a greeting\n")
	fmt.Fprint(w, "GET  /users/{id}   reads a path value\n")
	fmt.Fprint(w, "POST /echo         reports the request it was given\n")
	fmt.Fprint(w, "GET  /slow         holds the connection, and notices if you leave\n")
	fmt.Fprint(w, "GET  /info         runtime information\n")
}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "world"
	}
	fmt.Fprintf(w, "hello %s\n", name)
}

func showUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":   r.PathValue("id"),
		"path": r.URL.Path,
	})
}

func echoRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"method":     r.Method,
		"path":       r.URL.Path,
		"query":      r.URL.RawQuery,
		"user_agent": r.UserAgent(),
		"name":       r.FormValue("name"),
	})
}

// slow holds the connection and notices the client leaving, which is what
// connection_aborted() is for on the PHP side. Here it is the request context.
func slow(w http.ResponseWriter, r *http.Request) {
	ticks := 0
	for ticks < 20 {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-r.Context().Done():
		}
		ticks++
		if r.Context().Err() != nil {
			break
		}
	}
	if r.Context().Err() != nil {
		fmt.Fprintf(os.Stderr, "aborted after %d ticks, response skipped\n", ticks)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "waited %d ticks, still connected\n", ticks)
}

func info(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	fmt.Fprintf(w, "# go\n\nRuntime => net/http\nGo Version => %s\n", os.Getenv("GOVERSION"))
}

// bounded caps how many requests are answered at once, so a run can be put
// beside a PHP server holding the same number of workers.
func bounded(next http.Handler, workers int) http.Handler {
	if workers < 1 {
		return next
	}
	slots := make(chan struct{}, workers)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-r.Context().Done():
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	cfg := readConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.limit)
	defer cancel()

	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	server := &http.Server{Handler: bounded(routes(), cfg.workers)}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = server.Serve(listener)
	}()

	fmt.Printf("serving on http://%s until the time limit is up\n", listener.Addr())

	<-ctx.Done()

	grace, graceCancel := context.WithTimeout(context.Background(), time.Second)
	defer graceCancel()
	if err := server.Shutdown(grace); err != nil {
		_ = server.Close()
	}
	<-stopped
	_ = io.Discard
	fmt.Println("server stopped")
}
