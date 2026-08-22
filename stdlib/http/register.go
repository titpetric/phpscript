package http

import (
	"github.com/titpetric/phpscript/runner"
)

// Register installs the HTTP\Client and HTTP\Request bindings on rt.
func Register(rt *runner.Runtime) {
	// HTTP\Request is one outbound request: a method, a URL, and an optional body.
	// Building it sends nothing; a client does that with $client->send($request).
	rt.RegisterConstructor("HTTP\\Request", NewRequest)

	// HTTP\Client sends requests. It takes its settings as an associative array,
	// and with no argument gives a client with a 30 second timeout that follows
	// redirects.
	rt.RegisterConstructor("HTTP\\Client", NewClient)
}
