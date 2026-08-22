package http

import (
	"encoding/json"
	"fmt"
	nethttp "net/http"
)

// Response is the PHP-visible result of sending a request.
//
// This one is a facade rather than a net/http response, for two reasons. The
// body is read in full when the response is constructed, because a script has
// no stream to close and an unread net/http body leaks its connection. And
// ok() and json() are the two things a script does with a response that
// net/http has no equivalent for.
type Response struct {
	status int64
	header nethttp.Header
	body   string
	err    string
}

// Status returns the HTTP status code. It is an int, so it compares against a
// literal: $response->status() == 200. A request that never got a response
// reports 0; see err().
func (r *Response) Status() int64 { return r.status }

// OK reports whether the request got a response with a 2xx status.
func (r *Response) OK() bool { return r.err == "" && r.status >= 200 && r.status < 300 }

// Err returns why the request failed, or an empty string when it did not. Only
// parallel() produces a failed response: send() throws instead, because there
// is one outcome to report rather than several.
func (r *Response) Err() string { return r.err }

// Body returns the response body as a string.
func (r *Response) Body() string { return r.body }

// Header returns the value of the named response header, or an empty string
// when it is not set. Header names are matched case-insensitively.
func (r *Response) Header(name string) string { return r.header.Get(name) }

// Headers returns the response headers as an array of name to value. A header
// sent more than once is joined with ", ".
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
