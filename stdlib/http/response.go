package http

import (
	"encoding/json"
	"fmt"
	nethttp "net/http"
)

// Response is the PHP-visible result of sending a request. The body is read in
// full when the response is constructed, because a script has no stream to
// close; see Client.Send.
type Response struct {
	status int64
	header nethttp.Header
	body   string
}

// Status returns the HTTP status code. It is an int, so it compares against a
// literal: $response->status() == 200.
func (r *Response) Status() int64 { return r.status }

// OK reports whether the status is in the 2xx range.
func (r *Response) OK() bool { return r.status >= 200 && r.status < 300 }

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
