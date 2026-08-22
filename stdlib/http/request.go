package http

import (
	"context"
	"fmt"
	nethttp "net/http"
	"strings"
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
