package http

import (
	"context"
	"fmt"
	"net"
	nethttp "net/http"
	"sync"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// shutdownGrace bounds how long a graceful shutdown waits for the requests in
// flight. A script that ran out of time has no more to give them.
const shutdownGrace = time.Second

// Server listens and answers, which is what net/http's Server does and what a
// ServeMux does not. The vocabulary is Go's: listen(), then shutdown() to let
// the requests in flight finish, or close() to drop them.
type Server struct {
	rt      *runner.Runtime
	addr    string
	handler nethttp.Handler

	// mu guards the running server. listen starts it and shutdown stops it, and
	// a register_shutdown_function callback reaches the second from a different
	// place in the script than the first.
	mu      sync.Mutex
	server  *nethttp.Server
	bound   string
	stopped chan struct{}
}

// NewServer builds a server for $addr answering through $handler, which is an
// HTTP\Mux or anything else that answers a request. Building one listens on
// nothing: listen() does that.
//
// An $addr of "127.0.0.1:0" binds a port the system picks, which listen()
// answers with, and is how a test takes a free one rather than hoping.
func NewServer(rt *runner.Runtime, addr string, handler any) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("HTTP\\Server: addr is required")
	}
	routed, ok := handler.(nethttp.Handler)
	if !ok {
		return nil, fmt.Errorf("HTTP\\Server: handler must answer requests, %T does not", handler)
	}
	return &Server{rt: rt, addr: addr, handler: routed}, nil
}

// listen binds the address and starts answering, and returns the address it
// bound. It does not block: the script goes on, and calls wait() when it has
// nothing left to do.
func (s *Server) Listen() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return "", fmt.Errorf("HTTP\\Server::listen: already listening on %s", s.bound)
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", fmt.Errorf("HTTP\\Server::listen: %w", err)
	}
	// Held locally as well as on the struct: shutdown() takes the fields back
	// to nil the moment it is called, and this goroutine outlives that.
	server := &nethttp.Server{Handler: s.handler}
	stopped := make(chan struct{})
	s.server, s.stopped = server, stopped
	s.bound = listener.Addr().String()

	go func() {
		defer close(stopped)
		_ = server.Serve(listener)
	}()
	return s.bound, nil
}

// wait blocks until the script runs out of time, or the client that started it
// goes away, and returns. A script with no limit and no client waits until the
// host's own context ends, which for a command line run is never.
//
// Nothing runs after it in a script the time limit ended: the limit is a fatal,
// as php's is, so the next statement is where the script stops. Cleanup belongs
// in a register_shutdown_function callback, which runs with the clock off.
func (s *Server) Wait() {
	// Parked, not merely blocked: the script holds the runtime while it runs,
	// and its own handlers need it. ParkExec hands it back for exactly as long
	// as this waits.
	s.rt.ParkExec(func() { <-s.rt.Context().Done() })
}

// shutdown stops the server, letting the requests in flight finish first, and
// closing on whatever is still running after a second.
//
// It answers nothing and throws nothing. Calling it without having listened, or
// twice, is not an error, and neither is a request that would not finish:
// shutdown() is written into a register_shutdown_function callback, which runs
// when the script has already ended and has nowhere to put a failure. A handler
// calling it to stop its own server is the case that cannot finish gracefully -
// the request doing the asking is itself in flight - and it closes rather than
// hanging.
//
// The runtime is parked for the wait, so the requests being waited on can
// actually run.
func (s *Server) Shutdown() {
	server, stopped := s.take()
	if server == nil {
		return
	}
	s.rt.ParkExec(func() {
		grace, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := server.Shutdown(grace); err != nil {
			_ = server.Close()
		}
		<-stopped
	})
}

// close stops the server at once, dropping whatever was in flight. shutdown()
// is the one to reach for; this is for a script that has decided the answers no
// longer matter.
func (s *Server) Close() error {
	server, stopped := s.take()
	if server == nil {
		return nil
	}
	err := server.Close()
	<-stopped
	if err != nil {
		return fmt.Errorf("HTTP\\Server::close: %w", err)
	}
	return nil
}

// addr answers the address the server bound, empty until it has listened.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bound
}

// take claims the running server, so that shutdown and close each stop it once
// however often they are called and whichever of them arrives first.
func (s *Server) take() (*nethttp.Server, chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	server, stopped := s.server, s.stopped
	s.server, s.stopped, s.bound = nil, nil, ""
	return server, stopped
}
