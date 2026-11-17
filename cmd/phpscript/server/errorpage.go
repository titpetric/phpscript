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

// errorPageNames lists the files that may answer for status, in lookup order.
func errorPageNames(status int) [4]string {
	code := strconv.Itoa(status)
	return [4]string{code + ".php", code + ".html", "error.php", "error.html"}
}

// errorPage returns the file that answers for status, and whether one exists.
func (h *handler) errorPage(status int) (string, bool) {
	for _, name := range errorPageNames(status) {
		info, err := fs.Stat(h.public, name)
		if err == nil && !info.IsDir() {
			return name, true
		}
	}
	return "", false
}

// serveErrorPage renders the site's page for status, answering false for a non-browser request.
func (h *handler) serveErrorPage(w http.ResponseWriter, r *http.Request, status int, notes string) bool {
	return h.errorPageFor(w, r, status, notes, false)
}

// serveUnrouted renders the site's page for an unrouted request with its body intact.
func (h *handler) serveUnrouted(w http.ResponseWriter, r *http.Request, status int) bool {
	return h.errorPageFor(w, r, status, "", true)
}

// errorPageFor is the body of both; keepBody names whether a body remains.
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
		// A failed script has already read the body, which leaves no body to
		// hand on; the two headers go with it, or the page would advertise a
		// payload it cannot produce. $_POST and $_FILES are empty and
		// php://input has no bytes to read.
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

	// The page may answer with its own status; one that names none answers
	// with the status of the call.
	if staged := reqCtx.ResponseStatus(); staged != 0 {
		status = staged
	}
	reqCtx.SetDefaultHeader("Content-Type", "text/html; charset=utf-8")
	reqCtx.WriteResponse(w, status, body)
	return true
}

// serveErrorFile writes the named static page with status and reports whether it read one.
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

// redirectVars fills Apache's $_SERVER REDIRECT_* keys from the original request r.
func redirectVars(request runner.Context, r *http.Request, status int, notes string) {
	request.Server["REDIRECT_STATUS"] = strconv.Itoa(status)
	request.Server["REDIRECT_URL"] = r.URL.Path
	request.Server["REDIRECT_QUERY_STRING"] = r.URL.RawQuery
	if notes != "" {
		request.Server["REDIRECT_ERROR_NOTES"] = notes
	}
}
