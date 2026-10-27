package runner

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// AcceptsHTML reports whether an Accept header explicitly names HTML.
//
// Explicitly, and no wildcard. "*/*" is what curl and fetch() send and it
// matches every type there is and reading it as a request for HTML would put a
// website's error page in front of every program that talks to the server. Only
// text/html and application/xhtml+xml, written out and not weighted to zero,
// count; so does neither a "text/*" wildcard nor an absent header.
func AcceptsHTML(accept string) bool {
	for _, entry := range strings.Split(accept, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(entry))
		if err != nil {
			continue
		}
		if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
			continue
		}
		// A range the client weighted to zero is one it refuses, which is the
		// spelling for "anything but this".
		if weight, ok := params["q"]; ok {
			if q, err := strconv.ParseFloat(weight, 64); err == nil && q <= 0 {
				continue
			}
		}
		return true
	}
	return false
}

// WantsErrorPage reports whether a request is a browser navigation, the only
// kind of request a site's HTML error page answers. A HEAD request is never
// one: there is no body to render a page into.
//
// A URL prefix cannot answer this, because an API and a website share one URL
// space and one catch-all handler; the request answers it itself. Sec-Fetch-Dest
// is decisive when a browser sends it: fetch() and XHR send "empty", an image
// "image", a stylesheet "style", and only a navigation "document". A client that
// sends none falls to its Accept header, which has to name HTML explicitly.
func WantsErrorPage(r *http.Request) bool {
	if r.Method == http.MethodHead {
		return false
	}
	switch r.Header.Get("Sec-Fetch-Dest") {
	case "", "document", "iframe", "frame":
	default:
		return false
	}
	return AcceptsHTML(r.Header.Get("Accept"))
}

// statusCoder is the part of a PHP exception that carries a code:
// `throw new Exception("Not found", 404)`. Both stdlib.Exception and
// runner.RuntimeException have GetCode, and this file spells the interface
// structurally because stdlib imports runner and not the other way around.
type statusCoder interface{ GetCode() int }

// StatusFor resolves the HTTP status a finished script ends on, given whatever
// error it returned. Zero means nothing chose one and the host's default stands.
//
// An uncaught exception is the interesting case. Its code is the script's own
// number, which need not be a status at all and StatusFor takes only a code in
// the 4xx and 5xx range as one; anything else is an application code and the
// request failed with a 500. exit() and die() are not failures: a script that
// ended early still answers with the status it staged, as it does in PHP.
func (c Context) StatusFor(err error) int {
	if err == nil {
		return c.ResponseStatus()
	}
	if _, ok := IsExit(err); ok {
		return c.ResponseStatus()
	}
	var coder statusCoder
	if errors.As(err, &coder) {
		if code := coder.GetCode(); code >= 400 && code <= 599 {
			return code
		}
	}
	return http.StatusInternalServerError
}

// Answered reports whether the script answered this request itself.
func (c Context) Answered(body []byte) bool {
	return len(body) > 0 || c.response.Get("Content-Type") != ""
}

// WriteResponse flushes one response: the headers the script staged with
// header(), the status, and the body it produced. No byte reaches the
// ResponseWriter before this and a host can look at a finished
// response and answer with something else instead.
//
// A status of zero writes none, leaving net/http its 200. So does a status
// net/http will not send: it panics on anything outside 100 to 999, and
// http_response_code() takes whatever number a script passes.
func (c Context) WriteResponse(w http.ResponseWriter, status int, body []byte) {
	for name, values := range c.response {
		w.Header()[name] = values
	}
	if status >= 100 && status <= 999 {
		w.WriteHeader(status)
	}
	_, _ = w.Write(body)
}

// SetDefaultHeader stages a header only if the script did not set one itself.
// A host applies a default through it, such as PHP's text/html content type,
// without overriding what the script said.
func (c Context) SetDefaultHeader(name, value string) {
	if c.response.Get(name) == "" {
		c.response.Set(name, value)
	}
}

// Header implements PHP header($header[, $replace[, $code]]): it parses a
// "Name: value" line and stages it on the response header set. replace controls
// whether an existing header of the same name is overwritten (default true).
func (c Context) Header(header string, opts ...any) {
	name, value, ok := strings.Cut(header, ":")
	if !ok {
		// A status line, "HTTP/1.0 404 Not Found", is the other thing header()
		// takes. It has no name:value shape, and what it carries is the status.
		if code, ok := parseStatusLine(header); ok {
			*c.status = code
		}
		return
	}
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)

	replace := true
	if len(opts) > 0 {
		replace = phpTruthy(opts[0])
	}
	if replace {
		c.response.Set(name, value)
	} else {
		c.response.Add(name, value)
	}
	if strings.EqualFold(name, "Location") && c.ResponseStatus() == 0 {
		*c.status = http.StatusFound
	}
	// $code is an int parameter and PHP coerces whatever the call passes, and
	// zero is the value that means the call named no status.
	if len(opts) > 1 {
		if code := toInt(opts[1]); code != 0 {
			*c.status = int(code)
		}
	}
}

// parseStatusLine reads the status out of a "HTTP/1.0 404 Not Found" line, the
// spelling header() takes in place of a name and a value.
//
// Neither the protocol nor the reason phrase reaches the response. Go writes
// the protocol it actually serves and the reason phrase that belongs to the
// code and all the line can contribute is the number in the middle.
func parseStatusLine(header string) (int, bool) {
	if !strings.HasPrefix(header, "HTTP/") {
		return 0, false
	}
	fields := strings.Fields(header)
	if len(fields) < 2 {
		return 0, false
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil || code < 100 || code > 599 {
		return 0, false
	}
	return code, true
}

// HTTPResponseCode implements PHP http_response_code([$response_code]).
//
// With no argument it reports the status this response will carry, and with
// one it stages that status and reports the one it replaced.
//
// PHP answers false while nothing has set a status, and true for the first one
// set, having no previous number to return. On
// a web SAPI neither happens: a request starts out answering 200. sapi is the
// SAPI name the runtime runs under, and only the command line, or a host that
// named no SAPI at all, starts without a status.
func (c Context) HTTPResponseCode(sapi string, opts ...any) any {
	previous := c.ResponseStatus()
	if previous == 0 && sapi != "" && sapi != "cli" {
		previous = http.StatusOK
	}

	// The parameter is an int and PHP coerces whatever the call passes, and
	// $_SERVER holds every value as a string: a page passing REDIRECT_STATUS
	// straight back is passing "404" and means 404.
	code := 0
	if len(opts) > 0 {
		code = int(toInt(opts[0]))
	}
	// A zero, a null and a false are not a status. PHP treats the call as the
	// reporting one, which is all any of them amounts to here.
	if code == 0 {
		if previous == 0 {
			return false
		}
		return int64(previous)
	}

	*c.status = code
	if previous == 0 {
		return true
	}
	return int64(previous)
}

// ResponseHeaders returns the headers header() staged, for a host to copy.
func (c Context) ResponseHeaders() http.Header { return c.response }

// AddResponseHeader appends a header, for a Set-Cookie and its like.
func (c Context) AddResponseHeader(name, value string) {
	c.response.Add(name, value)
}

// ResponseStatus returns the status staged by header(), or zero when the host
// should keep its default status. A Location header defaults to 302 like PHP.
func (c Context) ResponseStatus() int {
	if c.status == nil {
		return 0
	}
	return *c.status
}
