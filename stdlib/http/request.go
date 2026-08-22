package http

import (
	"context"
	"fmt"
	nethttp "net/http"
	neturl "net/url"
	"strings"
)

// Request is the PHP-visible HTTP\Request. The net/http request is a named
// unexported field rather than an embedded one, so a script reaches the methods
// below and nothing else net/http exports.
type Request struct {
	method string
	url    *neturl.URL
	header nethttp.Header
	body   string
}

// NewRequest builds a request from $method and $url, with an optional $body.
// The method is upper-cased, so new HTTP\Request("get", $url) is a GET.
// Building a request sends nothing: pass it to a client with
// $client->send($request), and set headers on it with $request->set_header().
func NewRequest(ctx context.Context, method, url string, body ...string) (*Request, error) {
	if method == "" {
		return nil, fmt.Errorf("HTTP\\Request: method is required")
	}
	if url == "" {
		return nil, fmt.Errorf("HTTP\\Request: url is required")
	}
	parsed, err := neturl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP\\Request: %w", err)
	}

	request := &Request{
		method: strings.ToUpper(method),
		url:    parsed,
		header: nethttp.Header{},
	}
	if len(body) > 0 {
		request.body = body[0]
	}
	return request, nil
}

// Method returns the request method.
func (r *Request) Method() string { return r.method }

// URL returns the request URL, including any query parameters set on it.
func (r *Request) URL() string { return r.url.String() }

// Body returns the request body.
func (r *Request) Body() string { return r.body }

// Header returns the value of the named request header, or an empty string when
// it is not set. Header names are matched case-insensitively.
func (r *Request) Header(name string) string { return r.header.Get(name) }

// Headers returns the request headers as an array of name to value. A header
// set more than once is joined with ", ".
func (r *Request) Headers() map[string]string { return flattenHeader(r.header) }

// SetHeader sets a request header, replacing any value already set for it, and
// returns the request so calls can be chained.
func (r *Request) SetHeader(name, value string) *Request {
	r.header.Set(name, value)
	return r
}

// AddHeader adds a request header, keeping any value already set for it, and
// returns the request so calls can be chained.
func (r *Request) AddHeader(name, value string) *Request {
	r.header.Add(name, value)
	return r
}

// SetBody replaces the request body and returns the request so calls can be
// chained.
func (r *Request) SetBody(body string) *Request {
	r.body = body
	return r
}

// SetQuery sets a query parameter on the request URL, replacing any value
// already set for it, and returns the request so calls can be chained.
func (r *Request) SetQuery(name, value string) *Request {
	query := r.url.Query()
	query.Set(name, value)
	r.url.RawQuery = query.Encode()
	return r
}

// build turns the facade into the net/http request a client sends. It is not
// exported, so it is not reachable from a script.
func (r *Request) build(ctx context.Context, base string) (*nethttp.Request, error) {
	target := r.url
	if base != "" && !target.IsAbs() {
		parsed, err := neturl.Parse(base)
		if err != nil {
			return nil, fmt.Errorf("HTTP\\Client: base_url: %w", err)
		}
		target = parsed.ResolveReference(target)
	}

	var body *strings.Reader
	if r.body != "" {
		body = strings.NewReader(r.body)
	}

	// A nil *strings.Reader in an io.Reader interface is not a nil interface,
	// and net/http would then read from it and panic, so the two cases are
	// spelled separately rather than passed through one variable.
	var request *nethttp.Request
	var err error
	if body == nil {
		request, err = nethttp.NewRequestWithContext(ctx, r.method, target.String(), nil)
	} else {
		request, err = nethttp.NewRequestWithContext(ctx, r.method, target.String(), body)
	}
	if err != nil {
		return nil, fmt.Errorf("HTTP\\Client: %w", err)
	}
	for name, values := range r.header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	return request, nil
}

// flattenHeader renders a header set as the array shape a script reads. A
// map[string]string is the cheap shape for a value a script only reads; see
// docs/allocation-performance.md.
func flattenHeader(header nethttp.Header) map[string]string {
	out := make(map[string]string, len(header))
	for name, values := range header {
		out[name] = strings.Join(values, ", ")
	}
	return out
}
