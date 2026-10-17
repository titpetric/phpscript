package annotations

import (
	"bytes"
	"io"
	"io/fs"
	"log"
	"net/http"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/telemetry"
)

// handler executes routed PHP endpoints, one request at a time.
type handler struct {
	root         fs.FS
	config       config
	includeCache *runner.IncludeCache
	exprCache    *runner.ExprCache

	// pool holds this source tree's registered runtimes between requests. It
	// belongs to the handler the way the include cache does, so a runtime
	// registered against one tree's root and options can never answer a
	// request routed to another's; see runner.SessionPool.
	pool *runner.SessionPool
}

// newHandler creates the handler shared by every endpoint of one source tree.
func newHandler(root fs.FS, config config) *handler {
	exprCache := config.exprCache
	if exprCache == nil {
		exprCache = runner.NewExprCache()
	}
	// Include paths are relative to one filesystem root. A cache belongs to one
	// source tree, so two roots containing the same path cannot share a parsed
	// program accidentally; the host of this tree may pass the one it filled.
	includeCache := config.includeCache
	if includeCache == nil {
		includeCache = runner.NewIncludeCache()
	}
	h := &handler{
		root:         root,
		config:       config,
		includeCache: includeCache,
		exprCache:    exprCache,
	}
	h.pool = runner.NewSessionPool(0, h.buildRuntime)
	return h
}

// buildRuntime constructs one runtime of this tree's pool, the way newRuntime
// does for a startup job, minus everything per-request: the standard library,
// the tree's root, its caches and its observers, installed once and serving
// every request the pool hands it to. FreezeStdlib marks the constants and the
// environment as the host's, so a reset between requests keeps them and drops
// what an endpoint defined.
func (h *handler) buildRuntime() *runner.Runtime {
	options := h.config.runnerOptions
	options.RootFS = h.root
	options.SAPI = "http"

	construct := runner.New
	if h.config.flatstack {
		construct = runner.NewFlatStack
	}
	rt := construct(io.Discard, options)
	rt.SetIncludeCache(h.includeCache)
	rt.SetExprCache(h.exprCache)
	for _, observer := range h.config.observers {
		rt.Observe(observer)
	}
	stdlib.Register(rt)
	if h.config.rootDir != "" {
		stdlib.RegisterFS(rt, h.config.rootDir)
	}
	rt.FreezeStdlib()
	return rt
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

	// A pooled runtime, registered once by buildRuntime and reset between
	// requests: nothing a request wrote survives the Put. The response is a
	// pushed redirection, dropped with the rest of the session by the reset,
	// and the connection enters through BeginRequest the way a Path B worker's
	// does. The runtime hooks keep the order newRuntime gives a startup job:
	// the request context first, then the host's own functions over it.
	rt := h.pool.Get()
	defer h.pool.Put(rt)
	rt.PushOutput(&out)
	defer rt.EndRequest(rt.BeginRequest(r.Context()))
	rt.SetContext(r.Context())
	request.Register(rt)
	for _, fn := range h.config.runtimeFuncs {
		fn(rt)
	}
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
