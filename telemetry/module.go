package telemetry

import (
	"context"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"github.com/titpetric/platform"
)

// Module records the telemetry of a phpscript service. One value is three
// things at once, the way the service needs them: HTTP middleware recording
// every request, a platform module serving the debug front end, and a runtime
// observer forwarding what the PHP interpreter reports onto the trace of the
// request that is running.
type Module struct {
	platform.UnimplementedModule

	options    Options
	tracer     *Tracer
	middleware func(http.Handler) http.Handler
}

var _ platform.Module = (*Module)(nil)

// NewModule returns a telemetry module recording into its own tracer. The
// tracer is explicit rather than the process wide one, so two services, or two
// tests, do not record into each other.
func NewModule(options Options) (*Module, error) {
	options = options.WithDefaults()
	if options.RouteFunc == nil {
		options.RouteFunc = routePattern
	}
	tracer, err := New(options)
	if err != nil {
		return nil, err
	}
	options.Tracer = tracer

	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule("telemetry"),
		options:             options,
		tracer:              tracer,
		middleware:          TracingMiddleware(options),
	}, nil
}

// Mount registers the debug front end on the platform router.
func (m *Module) Mount(_ context.Context, r platform.Router) error {
	return Mount(r, m.options)
}

// Middleware records requests handled by next.
func (m *Module) Middleware(next http.Handler) http.Handler {
	return m.middleware(next)
}

// Options returns the options the module was built with, including the tracer
// it recorded into.
func (m *Module) Options() Options {
	return m.options
}

// Tracer returns the tracer the module records into.
func (m *Module) Tracer() *Tracer {
	return m.tracer
}

// Snapshot returns a race free copy of everything recorded so far.
func (m *Module) Snapshot() Snapshot {
	return m.tracer.Snapshot()
}

// TrackLifecycle records work that did not arrive over the network, such as a
// @startup file, as a trace of its own.
func (m *Module) TrackLifecycle(ctx context.Context, name, filename string, run func(context.Context) error) error {
	return m.tracer.Observe(ctx, name, func(ctx context.Context) error {
		trace := TraceFromContext(ctx)
		trace.Root().SetSource(filename, 0)

		err := run(WithSpanFilename(ctx, filename))
		if err != nil {
			trace.Root().RecordError(err)
			trace.SetState(StateError)
		}
		return err
	})
}

// UpdateStatus implements runner.Observer: it moves the trace of the running
// request to the scoreboard state the interpreter reports.
func (m *Module) UpdateStatus(ctx context.Context, state State) {
	TraceFromContext(ctx).SetState(state)
}

// Trace implements runner.Observer: it records one span of interpreter work,
// such as an include, a call or a template, on the running trace.
func (m *Module) Trace(ctx context.Context, message string, kind ...Kind) *Span {
	return StartSpan(ctx, message, kind...)
}

// UpdateFilename records the PHP entrypoint of the running request. Included
// files do not replace it: the entrypoint is the file the request resolved to.
func (m *Module) UpdateFilename(ctx context.Context, filename string) {
	if filename == "" {
		return
	}
	root := TraceFromContext(ctx).Root()
	root.SetAttribute("filename", filename)
	if root.SourceText() == "" {
		root.SetSource(filename, 0)
	}
}

// UpdateIncludedFiles records how many files the request included beyond its
// entrypoint.
func (m *Module) UpdateIncludedFiles(ctx context.Context, count int) {
	TraceFromContext(ctx).Root().SetAttribute("included_files", count)
}

// routePattern groups statistics by the routed pattern rather than the request
// URI, so /hello/Ada and /hello/Grace aggregate into GET /hello/{name}.
//
// The PHP file server is mounted on a catch-all, and every page it serves would
// group under it. That pattern says nothing, so it is dropped and those
// requests group by path, which is the PHP file that ran.
func routePattern(r *http.Request) string {
	routeContext := chi.RouteContext(r.Context())
	if routeContext == nil {
		return ""
	}
	switch pattern := routeContext.RoutePattern(); pattern {
	case "/*", "/", "":
		return ""
	default:
		return pattern
	}
}
