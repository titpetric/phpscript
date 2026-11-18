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
// Explicit HTML only. "*/*" is the header curl and fetch() send; it matches
// every type, and reading it as a request for HTML would put a website's error
// page in front of every program that talks to the server. Only text/html and
// application/xhtml+xml count, written out and not weighted to zero. A "text/*"
// wildcard and an absent header do not count either.
func AcceptsHTML(accept string) bool {
	for _, entry := range strings.Split(accept, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(entry))
		if err != nil {
			continue
		}
		if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
			continue
		}
		// The client refuses a range weighted to zero; that spelling means
		// "anything but this".
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
// request kind a site's HTML error page answers. A HEAD request never
// qualifies. Sec-Fetch-Dest decides; an absent header falls to Accept.
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

// statusCoder is the part of a PHP exception that holds a code, GetCode.
type statusCoder interface{ GetCode() int }

// StatusFor resolves the HTTP status a finished script ends on, given whatever
// error it returned. Zero means no code won; the host's default stands.
//
// Only an exception code in the 4xx or 5xx range counts as a status; anything
// else is an application code and the request failed with a 500. exit() and
// die() are not failures: the script answers with the status it staged.
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

// WriteResponse flushes one response: the staged headers, the status and the
// body. A status of zero writes none, leaving net/http its 200. A status
// outside 100 to 999 writes none as well; net/http panics on it.
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
		// takes. It has no name:value shape; the status is the number in it.
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

// parseStatusLine reads the status number from an "HTTP/1.0 404 Not Found" line.
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
// Called with no argument, it reports the status this response will send. With
// an argument it stages that status and reports the status it replaced.
//
// PHP answers false while nothing has set a status, and true for the first one
// set. On a web SAPI neither happens: a request starts out answering 200, and
// only the command line, or a host naming no SAPI, starts with none.
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
	// A zero, a null, or a false is not a status. PHP treats the call as the
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
