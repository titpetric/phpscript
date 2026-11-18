package http

import (
	"context"
	"fmt"
	nethttp "net/http"
	"strings"
)

// NewRequest builds a request from $method and $url, with an optional $body.
// Building it sends no bytes; $client->send($request) does.
//
// The value is a net/http request: $request->method, $request->host, and
// $request->header->set($name, $value) for headers. A lowercase method is
// upper-cased. phpscript never sends the request as written.
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

	// A nil *strings.Reader held in an io.Reader is not a nil io.Reader.
	// net/http would then read from a non-nil interface. The call below passes
	// the two cases separately, and keeps each case in a separate argument.
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

// flattenHeader renders a header set as map[string]string, joining repeated values with ", ".
func flattenHeader(header nethttp.Header) map[string]string {
	out := make(map[string]string, len(header))
	for name, values := range header {
		out[name] = strings.Join(values, ", ")
	}
	return out
}
