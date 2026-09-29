package http

import (
	"github.com/titpetric/phpscript/runner"
)

// Register installs the HTTP\Client and HTTP\Request bindings on rt.
func Register(rt *runner.Runtime) {
	// HTTP\Request is one outbound request. It is a net/http request, so a script
	// reads and writes it the way Go does: $request->method, $request->host,
	// $request->url->path, and $request->header->set($name, $value). Building one
	// sends nothing; a client does that with $client->send($request).
	rt.RegisterConstructor("HTTP\\Request", NewRequest)

	// HTTP\Client sends requests, one at a time with send() or all at once with
	// parallel(). It takes its settings as an associative array, and with no
	// argument gives a client with a 30 second timeout that follows redirects.
	rt.RegisterConstructor("HTTP\\Client", NewClient)

	// HTTP\Mux routes requests to the PHP functions that answer them. It is
	// net/http's ServeMux, so $mux->handle("GET /users/{id}", $fn) takes the
	// patterns Go takes, and a handler is called with the response writer and
	// the request themselves.
	rt.RegisterConstructor("HTTP\\Mux", func() *Mux { return NewMux(rt) })

	// HTTP\Server listens on $addr and answers through $handler, an HTTP\Mux or
	// anything else that answers a request. $server->listen() binds and returns
	// the address, $server->wait() blocks until the script runs out of time,
	// and $server->shutdown() stops it, letting what is in flight finish.
	rt.RegisterConstructor("HTTP\\Server", func(addr string, handler any) (*Server, error) {
		return NewServer(rt, addr, handler)
	})
}
