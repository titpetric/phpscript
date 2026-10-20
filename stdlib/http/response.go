package http

import (
	"encoding/json"
	"fmt"
	nethttp "net/http"
)

// Response is the PHP-visible result of sending a request.
//
// This one is a facade and no net/http response, for two reasons. The client
// reads the body in full as it builds the response, because a script has no
// stream to close and an unread net/http body leaks its connection. And ok()
// and json() are the two things a script does with a response that net/http
// has no counterpart for.
type Response struct {
	status int64
	header nethttp.Header
	body   string
	err    string
}

// Status returns the HTTP status code. The code is an int, so it compares
// against a literal: $response->status() == 200. A request that never got a
// response reports 0; see err().
func (r *Response) Status() int64 { return r.status }

// OK reports whether the request got a response with a 2xx status.
func (r *Response) OK() bool { return r.err == "" && r.status >= 200 && r.status < 300 }

// Err returns why the request failed, or an empty string when it did not. Only
// parallel() produces a failed response: send() throws instead, because it
// reports one outcome and never a list of them.
func (r *Response) Err() string { return r.err }

// Body returns the response body as a string.
func (r *Response) Body() string { return r.body }

// Header returns the value of the named response header, or an empty string
// when the response carries none. The lookup ignores case.
func (r *Response) Header(name string) string { return r.header.Get(name) }

// Headers returns the response headers as an array of name to value, joining
// a header sent more than once with ", ".
func (r *Response) Headers() map[string]string { return flattenHeader(r.header) }

// JSON decodes the response body and returns it as arrays and scalars. It
// throws when the body is not valid JSON.
func (r *Response) JSON() (any, error) {
	var decoded any
	if err := json.Unmarshal([]byte(r.body), &decoded); err != nil {
		return nil, fmt.Errorf("HTTP\\Response: json: %w", err)
	}
	return decoded, nil
}
