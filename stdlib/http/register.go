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
}
