package server

import (
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// errorPageNames returns the files that may answer for a status, in the order
// they are tried. They live in the document root and are named after the status
// they answer for, so a site puts one up by writing it and takes it down by
// deleting it. There is nothing to configure and nothing to switch on.
//
// The error pages come last so one page can cover the statuses a site does not
// care to distinguish. error.php reads the status it is answering for out of
// $_SERVER["REDIRECT_STATUS"]; error.html cannot name it, and is the one page a
// site of static files has to answer every failure with.
//
// The .php name is preferred at each step, so a site that adds a 404.php beside
// a hand written 404.html starts serving the new one by writing it.
//
// The names are not nested. A directory of its own for /api could only add a
// page, never remove one, so it cannot say "no page here", which is the thing
// an API actually wants. That is decided per request instead; see serveErrorPage.
func errorPageNames(status int) [4]string {
	code := strconv.Itoa(status)
	return [4]string{code + ".php", code + ".html", "error.php", "error.html"}
}

// errorPage returns the file that answers for status, named relative to the
// document root, and whether the site has one.
func (h *handler) errorPage(status int) (string, bool) {
	for _, name := range errorPageNames(status) {
		info, err := fs.Stat(h.public, name)
		if err == nil && !info.IsDir() {
			return name, true
		}
	}
	return "", false
}

// serveErrorPage renders the site's own page for status onto w and reports
// whether it did. A false answer leaves the response untouched for the caller
// to write what it would have written anyway.
//
// It answers false for anything but a browser navigation. The catch-all this
// server mounts means an unrouted /api/... request is indistinguishable from an
// unrouted /article/... one by its path, so the path is not what decides:
// runner.WantsErrorPage asks the request whether a person or a program is at
// the other end. A fetch(), an XHR, curl, an <img> and a stylesheet all get the
// plain status they got before this existed, and an API needs no configuration,
// no prefix and no opt-out to keep them.
//
// The page runs once. Its own failure is logged and answered false, so a broken
// 500.php cannot ask for a 500 page of its own.
//
// The request body is dropped, because every caller of this reached it through
// a script that had already read the body. serveUnrouted is the entry point for
// the other case.
func (h *handler) serveErrorPage(w http.ResponseWriter, r *http.Request, status int, notes string) bool {
	return h.errorPageFor(w, r, status, notes, false)
}

// serveUnrouted renders the site's own page for a request that resolved to
// nothing, and reports whether it did.
//
// It differs from serveErrorPage in one way: the page is handed the request
// whole, body included. Nothing has read that body — the request matched no
// file and ran no script — so a page answering here is the last thing that can,
// and a site whose 404.php dispatches its own routes needs it to. A form
// posting to a URL no file backs arrives with $_POST, $_FILES and php://input
// filled the way the endpoint it was meant for would have seen them, and the
// page may answer 200.
func (h *handler) serveUnrouted(w http.ResponseWriter, r *http.Request, status int) bool {
	return h.errorPageFor(w, r, status, "", true)
}

// errorPageFor is the body of both. keepBody says whether the request still has
// one to give.
func (h *handler) errorPageFor(w http.ResponseWriter, r *http.Request, status int, notes string, keepBody bool) bool {
	if !runner.WantsErrorPage(r) {
		return false
	}
	name, ok := h.errorPage(status)
	if !ok {
		return false
	}
	if path.Ext(name) != ".php" {
		return h.serveErrorFile(w, name, status)
	}

	// A page in a writable directory is content a visitor put there, the same
	// as any other .php below the document root. See handler.executes.
	entrypoint := path.Join(h.documentRoot, name)
	if !h.executes(entrypoint) {
		return false
	}

	page := r.Clone(r.Context())
	if !keepBody {
		// A failed script has already read the body, so there is nothing to
		// hand on; the two headers go with it, or the page would advertise a
		// payload it cannot produce. $_POST and $_FILES are empty and
		// php://input reads nothing.
		page.Body = http.NoBody
		page.ContentLength = 0
		page.Header.Del("Content-Length")
		page.Header.Del("Content-Type")
	}

	reqCtx, body, err := h.run(w, page, entrypoint, func(request runner.Context) {
		redirectVars(request, r, status, notes)
	})
	if _, exited := runner.IsExit(err); err != nil && !exited {
		log.Printf("Error in error page %s for request %s: %s [trace %s]", entrypoint, r.URL.Path, err, telemetry.TraceID(r.Context()))
		return false
	}

	// The page may answer with a status of its own; one that names none answers
	// with the status it was called for.
	if staged := reqCtx.ResponseStatus(); staged != 0 {
		status = staged
	}
	reqCtx.SetDefaultHeader("Content-Type", "text/html; charset=utf-8")
	reqCtx.WriteResponse(w, status, body)
	return true
}

// serveErrorFile answers with a static error page. http.FileServer is no use
// for one: it owns the status it writes, and the status is the whole point.
func (h *handler) serveErrorFile(w http.ResponseWriter, name string, status int) bool {
	body, err := fs.ReadFile(h.public, name)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	return true
}

// redirectVars names the error the page is answering for, in the $_SERVER keys
// Apache fills in for an ErrorDocument. They are Apache's names rather than
// names of phpscript's own because a page written against one server should
// work on the other.
//
// r is the original request, not the one the page runs under, so REDIRECT_URL
// is the path that failed rather than the path of the page.
func redirectVars(request runner.Context, r *http.Request, status int, notes string) {
	request.Server["REDIRECT_STATUS"] = strconv.Itoa(status)
	request.Server["REDIRECT_URL"] = r.URL.Path
	request.Server["REDIRECT_QUERY_STRING"] = r.URL.RawQuery
	if notes != "" {
		request.Server["REDIRECT_ERROR_NOTES"] = notes
	}
}
