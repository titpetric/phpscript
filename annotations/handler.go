package annotations

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

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

// file returns the HTTP handler that serves one annotated PHP endpoint.
func (h *handler) file(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serve(w, r, name)
	})
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request, file string) {
	var out strings.Builder
	request := runner.FromRequestOptions(r, h.config.runnerOptions)
	// Uploaded parts are copied to temporary files for the script to read; they
	// belong to this request and nothing outlives it.
	defer request.Cleanup()

	rt := h.config.newRuntime(r.Context(), h.root, &out, "http", func(rt *runner.Runtime) {
		rt.SetIncludeCache(h.includeCache)
		rt.SetExprCache(h.exprCache)
		request.Register(rt)
	})

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

	for name, values := range request.ResponseHeaders() {
		w.Header()[name] = values
	}
	if err != nil {
		if _, ok := runner.IsExit(err); !ok {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if status := request.ResponseStatus(); status != 0 {
		w.WriteHeader(status)
	}
	_, _ = io.WriteString(w, out.String())
}
