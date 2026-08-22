package runner

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// exception stands in for stdlib.Exception, which runner cannot import: stdlib
// imports runner, which is why StatusFor asks for the code structurally.
type exception struct {
	message string
	code    int
}

func (e *exception) Error() string { return e.message }

func (e *exception) GetCode() int { return e.code }

// TestAcceptsHTML pins that only an explicit request for HTML counts as one.
// "*/*" matches every type there is, and it is what curl and fetch() send, so
// reading it as a request for HTML would put a website's error page in front of
// every program that talks to the server.
func TestAcceptsHTML(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   bool
	}{
		{name: "chrome navigation", accept: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8", want: true},
		{name: "safari navigation", accept: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", want: true},
		{name: "bare html", accept: "text/html", want: true},
		{name: "xhtml only", accept: "application/xhtml+xml", want: true},
		{name: "html with a weight", accept: "application/json, text/html;q=0.1", want: true},

		{name: "curl", accept: "*/*", want: false},
		{name: "empty", accept: "", want: false},
		{name: "json api", accept: "application/json", want: false},
		{name: "jquery ajax", accept: "application/json, text/javascript, */*; q=0.01", want: false},
		{name: "image", accept: "image/avif,image/webp,*/*", want: false},
		{name: "stylesheet", accept: "text/css,*/*;q=0.1", want: false},
		{name: "text wildcard is not html", accept: "text/*", want: false},
		{name: "html refused", accept: "application/json, text/html;q=0", want: false},
		{name: "unparseable", accept: "text/html;;;", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := AcceptsHTML(test.accept); got != test.want {
				t.Fatalf("AcceptsHTML(%q) = %v, want %v", test.accept, got, test.want)
			}
		})
	}
}

// TestWantsErrorPage pins that Sec-Fetch-Dest decides when a browser sends it.
// It is the one signal that separates a navigation from a fetch() the page made
// on its own behalf, which Accept cannot do: both can ask for HTML.
func TestWantsErrorPage(t *testing.T) {
	const navigation = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

	tests := []struct {
		name   string
		method string
		accept string
		dest   string
		want   bool
	}{
		{name: "navigation", accept: navigation, want: true},
		{name: "labelled navigation", accept: navigation, dest: "document", want: true},
		{name: "framed navigation", accept: navigation, dest: "iframe", want: true},

		{name: "fetch asking for html", accept: navigation, dest: "empty", want: false},
		{name: "image", accept: navigation, dest: "image", want: false},
		{name: "script", accept: navigation, dest: "script", want: false},
		{name: "head", method: http.MethodHead, accept: navigation, want: false},
		{name: "curl", accept: "*/*", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			r := httptest.NewRequest(method, "/missing", nil)
			r.Header.Set("Accept", test.accept)
			if test.dest != "" {
				r.Header.Set("Sec-Fetch-Dest", test.dest)
			}
			if got := WantsErrorPage(r); got != test.want {
				t.Fatalf("WantsErrorPage() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestStatusFor pins how an uncaught exception becomes a status. Only a code in
// the 4xx and 5xx range is one; every other code is the script's own numbering,
// and a request that ended on one simply failed.
func TestStatusFor(t *testing.T) {
	tests := []struct {
		name   string
		staged int
		err    error
		want   int
	}{
		{name: "nothing chosen", want: 0},
		{name: "staged", staged: 201, want: 201},
		{name: "exit keeps the staged status", staged: 404, err: &ExitError{}, want: 404},
		{name: "exit with a code is not a failure", err: &ExitError{Code: 1}, want: 0},

		{name: "http code", err: &exception{"gone", 503}, want: 503},
		{name: "http code at the edges", err: &exception{"bad request", 400}, want: 400},
		{name: "application code", err: &exception{"no such row", 2}, want: 500},
		{name: "no code", err: &exception{"boom", 0}, want: 500},
		{name: "code out of range", err: &exception{"teapot", 600}, want: 500},
		{name: "wrapped", err: errors.Join(&exception{"gone", 410}), want: 410},
		{name: "plain go error", err: errors.New("parse failed"), want: 500},

		// A failure overrides whatever the script staged before it: the status
		// describes how the request ended, not how it was going.
		{name: "failure after a staged status", staged: 200, err: &exception{"boom", 0}, want: 500},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := NewContext()
			if test.staged != 0 {
				*c.status = test.staged
			}
			if got := c.StatusFor(test.err); got != test.want {
				t.Fatalf("StatusFor() = %d, want %d", got, test.want)
			}
		})
	}
}

// TestAnswered pins the opt-out an API relies on. Either a body or a
// Content-Type is the script saying what its answer is, and neither has
// anything to do with where the script sits in the URL space.
func TestAnswered(t *testing.T) {
	c := NewContext()
	if c.Answered(nil) {
		t.Fatal("a script that said nothing has not answered")
	}
	if !c.Answered([]byte(`{"error":"no such user"}`)) {
		t.Fatal("a payload is an answer")
	}

	c = NewContext()
	c.Header("Content-Type: application/json")
	if !c.Answered(nil) {
		t.Fatal("a declared content type is an answer")
	}
}

// TestWriteResponse pins that a response is flushed in one place: the headers
// the script staged, then the status, then the body. A status of zero writes
// none and leaves net/http its 200.
func TestWriteResponse(t *testing.T) {
	c := NewContext()
	c.Header("X-Powered-By: phpscript")

	rr := httptest.NewRecorder()
	c.WriteResponse(rr, http.StatusTeapot, []byte("short and stout"))

	if rr.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Header().Get("X-Powered-By") != "phpscript" {
		t.Fatalf("headers = %v", rr.Header())
	}
	if rr.Body.String() != "short and stout" {
		t.Fatalf("body = %q", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	NewContext().WriteResponse(rr, 0, []byte("ok"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want the default", rr.Code)
	}
}

// TestWriteResponseRefusesAStatusThatCannotBeSent pins that a script cannot
// bring the handler down by naming a number. http_response_code() takes any
// integer and net/http panics on one outside 100 to 999, so a status that
// cannot be sent is treated as no status at all.
func TestWriteResponseRefusesAStatusThatCannotBeSent(t *testing.T) {
	for _, status := range []int{-1, 1, 99, 1000, 1 << 20} {
		rr := httptest.NewRecorder()
		NewContext().WriteResponse(rr, status, []byte("ok"))
		if rr.Code != http.StatusOK {
			t.Fatalf("WriteResponse(%d) wrote status %d, want the default", status, rr.Code)
		}
		if rr.Body.String() != "ok" {
			t.Fatalf("WriteResponse(%d) body = %q", status, rr.Body.String())
		}
	}
}

// TestSetDefaultHeader pins that a default never overrides what the script said.
func TestSetDefaultHeader(t *testing.T) {
	c := NewContext()
	c.SetDefaultHeader("Content-Type", "text/html; charset=utf-8")
	if got := c.ResponseHeaders().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}

	c = NewContext()
	c.Header("Content-Type: application/json")
	c.SetDefaultHeader("Content-Type", "text/html; charset=utf-8")
	if got := c.ResponseHeaders().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want the script's own", got)
	}
}
