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
// Explicitly is the whole point. "*/*" is what curl and fetch() send and it
// matches every type there is, so reading it as a request for HTML would put a
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
		// A range the client weighted to zero is one it is refusing, which is
		// the spelling for "anything but this".
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
// kind of request a site's HTML error page is meant for. A HEAD request is
// never one: there is no body to render a page into.
//
// A URL prefix cannot answer this, because an API and a website share one URL
// space and one catch-all handler; the request answers it itself. Sec-Fetch-Dest
// is decisive when a browser sends it: fetch() and XHR send "empty", an image
// "image", a stylesheet "style", and only a navigation "document". A client that
// sends none is judged by whether its Accept header explicitly names HTML.
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
// runner.RuntimeException satisfy it, and it is spelled structurally here
// because stdlib imports runner and not the other way around.
type statusCoder interface{ GetCode() int }

// StatusFor resolves the HTTP status a finished script ends on, given whatever
// error it returned. Zero means nothing chose one and the host's default stands.
//
// An uncaught exception is the interesting case. Its code is the script's own
// number and usually not a status at all, so only a code in the 4xx and 5xx
// range is taken as one; anything else is an application code and the request
// failed with a 500. exit() and die() are not failures: a script that ended
// early still answers with the status it staged, as it does in PHP.
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

// Answered reports whether the script answered this request itself, which a
// host takes as the script refusing whatever error page the site put up.
//
// A body counts, because a script that echoed something has said what the
// response is: an endpoint returning 404 with a JSON payload keeps it. So does
// a Content-Type, because a script that declared what it answers with has
// declared that it is not answering in HTML, which is a one line opt-out for an
// endpoint with nothing to put in the body. Neither has anything to do with
// where the script sits in the URL space, which is the point.
func (c Context) Answered(body []byte) bool {
	return len(body) > 0 || c.response.Get("Content-Type") != ""
}

// WriteResponse flushes one response: the headers the script staged with
// header(), the status, and the body it produced. Nothing reaches the
// ResponseWriter before this, which is what lets a host look at a finished
// response and answer with something else instead.
//
// A status of zero writes none, leaving net/http its 200. So does a status
// net/http will not send: it panics on anything outside 100 to 999, and
// http_response_code() takes whatever number a script hands it.
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
// It is how a host applies a default, such as PHP's text/html content type,
// without overriding what the script said.
func (c Context) SetDefaultHeader(name, value string) {
	if c.response.Get(name) == "" {
		c.response.Set(name, value)
	}
}
