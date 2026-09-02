package annotations

import (
	"bytes"
	"io/fs"
	"log"
	"net/http"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// handler executes routed PHP endpoints, one request at a time.
type handler struct {
	root         fs.FS
	config       config
	includeCache *runner.IncludeCache
	exprCache    *runner.ExprCache
}

// newHandler creates the handler shared by every endpoint of one source tree.
func newHandler(root fs.FS, config config) *handler {
	exprCache := config.exprCache
	if exprCache == nil {
		exprCache = runner.NewExprCache()
	}
	return &handler{
		root:   root,
		config: config,
		// Include paths are relative to one filesystem root. Keep a cache per
		// source tree so two roots containing the same path cannot share a
		// parsed program accidentally.
		includeCache: runner.NewIncludeCache(),
		exprCache:    exprCache,
	}
}

// route returns the HTTP handler that serves one annotated PHP endpoint.
//
// pattern is the path as the annotation declared it, before either router's
// rewriting, and rides the request context so $_REQUEST can name its path
// parameters the way the author wrote them.
func (h *handler) route(name, pattern string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serve(w, r.WithContext(runner.WithRoutePattern(r.Context(), pattern)), name)
	})
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request, file string) {
	var out bytes.Buffer
	request := runner.FromRequestOptions(r, h.config.runnerOptions)
	// Uploaded parts are copied to temporary files for the script to read; they
	// belong to this request and nothing outlives it.
	defer request.Cleanup()

	rt := h.config.newRuntime(r.Context(), h.root, &out, "http", func(rt *runner.Runtime) {
		rt.SetIncludeCache(h.includeCache)
		rt.SetExprCache(h.exprCache)
		request.Register(rt)
	})
	defer h.config.cover(rt)()

	// The parsed request and the response writer live for this request too;
	// Register accounted the Context, these are the host structures around it.
	rt.AccountRequest(r, w)

	program, err := rt.LoadFile(file)
	if err == nil {
		err = rt.Run(program)
	}

	if trace := telemetry.TraceFromContext(r.Context()); trace != nil {
		// The script's frames are gone once Run returns, so the peak is the
		// request's memory footprint; a fresh usage walk would be baseline.
		trace.Root().SetAttribute("memory_usage", rt.MemoryPeak())
		if rt.MemoryLimit() > 0 {
			trace.Root().SetAttribute("memory_limit", rt.MemoryLimit().Bytes())
		}
	}

	status := request.StatusFor(err)
	body := out.Bytes()

	// What went wrong, for a host error page to do as it likes with. It is set
	// only for a failure, and a failure discards whatever the endpoint echoed
	// before it: half an answer is not one.
	var notes string
	if _, exited := runner.IsExit(err); err != nil && !exited {
		// The trace ID is also the Request-Id header, so a log line and the
		// recorded trace of the same request find each other.
		log.Printf("Error in request %s, %s: %s [trace %s]", r.URL.Path, file, err, telemetry.TraceID(r.Context()))
		notes, body = err.Error(), nil
	}

	// An endpoint that answered for itself keeps its answer. Answered is what an
	// API relies on to stay an API: a payload, or a Content-Type, and the host's
	// HTML page stays out of it.
	if h.config.errorPages != nil && status >= 400 && !request.Answered(body) &&
		h.config.errorPages(w, r, status, notes) {
		return
	}
	if notes != "" {
		// No page, so the status is the whole answer. The detail is in the log
		// and on the trace, not in a body the client reads.
		http.Error(w, http.StatusText(status), status)
		return
	}
	request.WriteResponse(w, status, body)
}
