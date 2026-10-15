package http

import (
	nethttp "net/http"
	"testing"
)

// TestResponse_Headers covers the whole header set a script walks with foreach.
// A PHP array holds one value per key, so a header sent more than once is
// joined with ", " and never dropped: Set-Cookie arrives twice on a login
// response, and a script reading the array has to see both.
//
// Header() answers one name case-insensitively through net/http's canonical
// form. This returns the map as the response carries it, so the keys are the
// canonical spellings and a script indexes them the way a browser prints them.
func TestResponse_Headers(t *testing.T) {
	t.Parallel()
	header := nethttp.Header{}
	header.Add("Content-Type", "application/json")
	header.Add("Set-Cookie", "a=1")
	header.Add("Set-Cookie", "b=2")

	response := &Response{status: 200, header: header, body: "{}"}

	got := response.Headers()
	for name, want := range map[string]string{
		"Content-Type": "application/json",
		"Set-Cookie":   "a=1, b=2",
	} {
		if got[name] != want {
			t.Errorf("Headers()[%q] = %q, want %q", name, got[name], want)
		}
	}
	if len(got) != 2 {
		t.Errorf("Headers() holds %d names (%v), want 2", len(got), got)
	}

	// The single-name accessor reads the same values, case-insensitively.
	if value := response.Header("content-type"); value != "application/json" {
		t.Errorf("Header(content-type) = %q, want application/json", value)
	}

	// A response that carries no headers answers an empty array, so foreach
	// over it runs zero times and never fails.
	empty := (&Response{header: nethttp.Header{}}).Headers()
	if len(empty) != 0 {
		t.Errorf("Headers() on a response with no headers = %v, want empty", empty)
	}
}

// TestResponseStatusAndOK covers how a script branches on an outcome. OK() is
// true only for a 2xx that arrived, so a 404 and a transport failure are both
// false, and Status() reports 0 for a request that got no response at all.
func TestResponseStatusAndOK(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		response *Response
		status   int64
		ok       bool
	}{
		{name: "200", response: &Response{status: 200}, status: 200, ok: true},
		{name: "299", response: &Response{status: 299}, status: 299, ok: true},
		{name: "302", response: &Response{status: 302}, status: 302},
		{name: "404", response: &Response{status: 404}, status: 404},
		{name: "500", response: &Response{status: 500}, status: 500},
		{name: "no response", response: &Response{err: "connection refused"}},
		{name: "2xx with a failure", response: &Response{status: 200, err: "read timeout"}, status: 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.response.Status(); got != test.status {
				t.Errorf("Status() = %d, want %d", got, test.status)
			}
			if got := test.response.OK(); got != test.ok {
				t.Errorf("OK() = %t, want %t", got, test.ok)
			}
		})
	}
}

// TestResponseBodyAndErr covers the two fields a failed request is read
// through. Only parallel() produces a failed response, so Err() is empty on
// everything send() returns, and the body is a string the script can print
// whatever the status.
func TestResponseBodyAndErr(t *testing.T) {
	t.Parallel()
	arrived := &Response{status: 404, header: nethttp.Header{}, body: "not found"}
	if got := arrived.Body(); got != "not found" {
		t.Errorf("Body() = %q, want %q", got, "not found")
	}
	if got := arrived.Err(); got != "" {
		t.Errorf("Err() = %q on a response that arrived, want empty", got)
	}

	failed := &Response{err: "dial tcp: connection refused"}
	if got := failed.Err(); got != "dial tcp: connection refused" {
		t.Errorf("Err() = %q, want the reason", got)
	}
	if got := failed.Body(); got != "" {
		t.Errorf("Body() = %q on a request that got no response, want empty", got)
	}
}

// TestResponseJSON covers decoding the body into arrays and scalars. A body
// that is not JSON throws, so a script never reads a zero value it would then
// treat as data.
func TestResponseJSON(t *testing.T) {
	t.Parallel()
	decoded, err := (&Response{body: `{"id":1,"tags":["a","b"]}`}).JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("JSON() = %#v, want a map", decoded)
	}
	if object["id"] != float64(1) {
		t.Errorf("id = %#v, want 1", object["id"])
	}
	if tags, ok := object["tags"].([]any); !ok || len(tags) != 2 || tags[0] != "a" {
		t.Errorf("tags = %#v, want [a b]", object["tags"])
	}

	for _, body := range []string{"", "not json", "{"} {
		if _, err := (&Response{body: body}).JSON(); err == nil {
			t.Errorf("JSON() accepted %q", body)
		}
	}
}
