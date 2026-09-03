// Command plugin replaces the standard library's HTTP\Client and HTTP\Request
// bindings with logging ones.
//
// It exists to show that a Go plugin, and not the standard library, served a
// construction. Bind runs after stdlib.Register, so registering the same names
// replaces what is already there; with "log" => true the constructor writes a
// marker line the fixture asserts, and the standard library emits no such
// line. That makes the fixture a direct proof the plugin ran rather than a
// check that something did.
//
// Build:
//
//	CGO_ENABLED=1 go build -buildmode=plugin -o plugin.so .
//
// The build passes neither -trimpath nor -ldflags, because neither `go test`
// nor `go install .` does, and a plugin has to match the binary that opens it.
//
// Nothing here imports phpscript. Host is declared below and satisfied
// structurally, so this plugin keeps working across a phpscript rebuild. The
// cost is that it cannot name phpscript's types either: these Request and
// Client types are the plugin's own, which is why it has to replace both names
// together. A plugin client cannot send a standard library request.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync/atomic"
)

// Host is what this plugin needs from the runtime. Go compares interface types
// structurally, so *runner.Runtime satisfies this without either side knowing
// the other's name, and this file links no phpscript package.
type Host interface {
	RegisterConstructor(name string, ctor any)
	Output() io.Writer
}

// requests counts constructions for the life of the process, so Init owns
// something a per-request Bind must not reset. It is reported through a method
// rather than written into the marker line: the count depends on how many
// fixtures ran before this one and on which engines, so it is not something a
// compared output can assert.
var requests atomic.Int64

// Init runs once per process. It resets the counter, which is the one piece of
// state this plugin has, and touches no runtime: Init is handed whichever
// runtime opened the plugin first, and the harness builds two per fixture.
func Init(ctx context.Context) error {
	requests.Store(0)
	return nil
}

// Bind installs this plugin's constructors on the runtime serving the current
// request, replacing the standard library's HTTP\Request and HTTP\Client.
//
// It registers constructors and no functions, deliberately. RegisterConstructor
// writes a map entry; RegisterFunc bumps the runtime's function generation,
// which invalidates its expression type environment, its compile configuration
// and every pooled evaluation environment. A per-request Bind that called
// RegisterFunc would undo the runtime's caching on every request.
func Bind(ctx context.Context, h Host) error {
	h.RegisterConstructor("HTTP\\Request", newRequest(h))
	h.RegisterConstructor("HTTP\\Client", newClient(h))
	return nil
}

// Request is the plugin's replacement for HTTP\Request.
type Request struct {
	method  string
	url     string
	body    string
	headers map[string]string
	logging bool
}

// newRequest returns the constructor registered for HTTP\Request. It closes
// over the Host rather than over h.Output(): ResetSession swaps the runtime's
// writer between runs and output buffering pushes another on top of it, so a
// writer captured at Bind time would write into the previous run's buffer.
func newRequest(h Host) func(ctx context.Context, method, url string, options ...any) (*Request, error) {
	return func(ctx context.Context, method, url string, options ...any) (*Request, error) {
		if method == "" {
			return nil, fmt.Errorf("HTTP\\Request: method is required")
		}
		if url == "" {
			return nil, fmt.Errorf("HTTP\\Request: url is required")
		}

		request := &Request{
			method:  strings.ToUpper(method),
			url:     url,
			headers: map[string]string{},
		}
		// The trailing argument is either a body, as the standard library
		// takes, or an options array. Accepting both keeps a script that
		// swapped the plugin in working unchanged.
		for _, option := range options {
			switch value := option.(type) {
			case string:
				request.body = value
			default:
				if flag(option, "log") {
					request.logging = true
				}
			}
		}

		n := requests.Add(1)
		if request.logging {
			// The runtime's own output is what a fixture compares, so this is
			// the line that proves the plugin served the call.
			fmt.Fprintf(h.Output(), "plugin: HTTP\\Request %s %s\n", request.method, request.url)
			// The log goes to stderr, which the comparison does not read. It
			// carries the counter, which a compared output could not.
			log.Printf("plugin/http: request %s %s #%d", request.method, request.url, n)
		}
		return request, nil
	}
}

// Method returns the request method.
func (r *Request) Method() string { return r.method }

// URL returns the request URL.
func (r *Request) URL() string { return r.url }

// Body returns the request body.
func (r *Request) Body() string { return r.body }

// Header returns a request header, or an empty string when it is not set.
func (r *Request) Header(name string) string { return r.headers[name] }

// SetHeader sets a request header and returns the request so calls chain.
func (r *Request) SetHeader(name, value string) *Request {
	r.headers[name] = value
	return r
}

// Driver reports which implementation served this request. The standard
// library has no such method, so a script can tell the two apart.
func (r *Request) Driver() string { return "plugin" }

// Count returns how many requests this plugin has constructed since the
// process started.
func (r *Request) Count() int64 { return requests.Load() }

// Client is the plugin's replacement for HTTP\Client. It sends nothing: the
// plugin exists to show which code served a construction, and a fixture that
// reached the network would fail whenever the network did.
type Client struct {
	logging bool
	timeout int64
}

// newClient returns the constructor registered for HTTP\Client.
func newClient(h Host) func(ctx context.Context, options ...any) (*Client, error) {
	return func(ctx context.Context, options ...any) (*Client, error) {
		client := &Client{}
		for _, option := range options {
			if flag(option, "log") {
				client.logging = true
			}
			if value, ok := lookup(option, "timeout"); ok {
				client.timeout = asInt(value)
			}
		}
		if client.logging {
			fmt.Fprintf(h.Output(), "plugin: HTTP\\Client timeout=%d\n", client.timeout)
			log.Printf("plugin/http: client timeout=%d", client.timeout)
		}
		return client, nil
	}
}

// Driver reports which implementation served this client.
func (c *Client) Driver() string { return "plugin" }

// Timeout returns the configured timeout in seconds.
func (c *Client) Timeout() int64 { return c.timeout }

// Send reports that this client does not send. The plugin replaces the
// standard library to show which code ran, not to reimplement it.
func (c *Client) Send(ctx context.Context, request any) (any, error) {
	return nil, fmt.Errorf("HTTP\\Client: the logging plugin does not send requests")
}

// flag reads a boolean option out of whatever a script passed as the options
// argument. The plugin cannot name phpscript's array type, so it reads the
// shapes that reach it through the bridge: a Go map, and anything exposing the
// key/value pairs as one.
func flag(options any, name string) bool {
	value, ok := lookup(options, name)
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "0", "false", "off", "no":
			return false
		}
		return true
	default:
		return asInt(value) != 0
	}
}

// rangeable is a PHP array as this plugin sees it. A PHP array reaches a
// binding as phpscript's own array type, which the plugin cannot name without
// linking phpscript; it can name the method, and Go satisfies an interface
// structurally, so this matches without an import. It is the same mechanism as
// Host, applied to a value rather than to the runtime.
type rangeable interface {
	Range(fn func(key, val any) bool)
}

// lookup reads one option by name, matched case-insensitively. It handles both
// shapes an options argument arrives as: a PHP array, and a Go map from a
// caller inside Go.
func lookup(options any, name string) (any, bool) {
	var found any
	var ok bool

	switch pairs := options.(type) {
	case rangeable:
		pairs.Range(func(key, val any) bool {
			if strings.EqualFold(asString(key), name) {
				found, ok = val, true
				return false
			}
			return true
		})
	case map[string]any:
		for key, val := range pairs {
			if strings.EqualFold(key, name) {
				return val, true
			}
		}
	}
	return found, ok
}

// asString renders an array key as a string, which is all a name comparison
// needs.
func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

// asInt reads an integer option.
func asInt(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	}
	return 0
}
