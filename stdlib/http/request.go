package http

import (
	"context"
	"fmt"
	nethttp "net/http"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// NewRequest builds a request from $method and $url, with an optional $body.
// Building a request sends nothing: pass it to a client with
// $client->send($request).
//
// The value is a net/http request, so a script reads and writes it the way Go
// does: $request->method, $request->host, and $request->header->set($name,
// $value) for headers. Methods are written uppercase, "GET" and "POST"; a
// lowercase one is upper-cased rather than sent as written, because a server
// treats the method as case-sensitive and would reject it.
func NewRequest(ctx context.Context, method, url string, body ...string) (*nethttp.Request, error) {
	if method == "" {
		return nil, fmt.Errorf("HTTP\\Request: method is required")
	}
	if url == "" {
		return nil, fmt.Errorf("HTTP\\Request: url is required")
	}

	var reader *strings.Reader
	if len(body) > 0 && body[0] != "" {
		reader = strings.NewReader(body[0])
	}

	// A nil *strings.Reader held in an io.Reader is not a nil io.Reader, and
	// net/http would then read from it, so the two cases are passed separately
	// rather than through one variable.
	var (
		request *nethttp.Request
		err     error
	)
	if reader == nil {
		request, err = nethttp.NewRequestWithContext(ctx, strings.ToUpper(method), url, nil)
	} else {
		request, err = nethttp.NewRequestWithContext(ctx, strings.ToUpper(method), url, reader)
	}
	if err != nil {
		return nil, fmt.Errorf("HTTP\\Request: %w", err)
	}
	return request, nil
}

// CurrentRequest returns the request being served, or null when nothing is
// being served. It is the same net/http request an outbound one is, so the
// inbound and the outbound side of HTTP read alike.
//
// The request is the host's own value rather than a copy of it. It is
// request-lived and reached by one script, so no lock guards it, and a write
// reaches only the code that runs after the write: net/http has already read
// what it needed by the time a script can see it.
//
// The return type is any rather than *http.Request so that "no request" is
// PHP's null. A nil pointer in a typed return slot is not a nil interface: it
// arrives as a value that is_null() answers false for and that a plain if()
// takes as true, which is the opposite of what the caller asked.
func CurrentRequest(ctx context.Context) any {
	c, ok := runner.RequestContext(ctx)
	if !ok {
		return nil
	}
	request := c.Request()
	if request == nil {
		return nil
	}
	return request
}

// flattenHeader renders a header set as the array shape a script reads. A
// header sent more than once is joined with ", ". map[string]string is the
// cheap shape for a value a script only reads; see
// docs/allocation-performance.md.
func flattenHeader(header nethttp.Header) map[string]string {
	out := make(map[string]string, len(header))
	for name, values := range header {
		out[name] = strings.Join(values, ", ")
	}
	return out
}
