package telemetry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chi "github.com/go-chi/chi/v5"
)

// newTestModule stands in for the host: it builds the recorder the platform
// would build and hands the module its tracer.
func newTestModule(t *testing.T) (*Module, Options) {
	t.Helper()

	options := NewOptions()
	options.ServiceName = "phpscript"
	options.RouteFunc = func(r *http.Request) string {
		if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
			return routeContext.RoutePattern()
		}
		return ""
	}
	tracer, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	options.Tracer = tracer
	return NewModule(tracer), options
}

func TestModuleRecordsRequestsAndServesFrontEnd(t *testing.T) {
	module, options := newTestModule(t)

	router := chi.NewRouter()
	router.Use(TracingMiddleware(options))
	if err := Mount(router, options); err != nil {
		t.Fatal(err)
	}
	router.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Whatever the request runs is instrumented through the context, the
		// same way a PHP binding below the interpreter is.
		StartSpan(r.Context(), "load user", KindDatabase).End()
		_, _ = w.Write([]byte("user"))
	})

	recorded := httptest.NewRecorder()
	router.ServeHTTP(recorded, httptest.NewRequest(http.MethodGet, "/users/1", nil))
	if recorded.Code != http.StatusOK {
		t.Fatalf("status = %d", recorded.Code)
	}
	id := recorded.Header().Get(RequestIDHeader)
	if id == "" {
		t.Fatal("Request-Id header is empty")
	}

	traces := module.Tracer().Traces()
	if len(traces) != 1 {
		t.Fatalf("traces = %+v", traces)
	}

	// A routed request groups by its pattern, so /users/1 and /users/2 are one
	// row in the statistics rather than two.
	trace := traces[0]
	if trace.Name != "GET /users/{id}" || trace.HTTP.Route != "/users/{id}" {
		t.Fatalf("trace = %+v", trace)
	}
	if len(trace.Spans) != 2 || trace.Spans[1].Name != "load user" || trace.Spans[1].Kind != KindDatabase {
		t.Fatalf("spans = %+v", trace.Spans)
	}

	front := httptest.NewRecorder()
	router.ServeHTTP(front, httptest.NewRequest(http.MethodGet, DefaultPath+"/traces", nil))
	if front.Code != http.StatusOK || !strings.Contains(front.Body.String(), DefaultPath+"/trace/"+id) {
		t.Fatalf("front end status = %d, body = %q", front.Code, front.Body.String())
	}

	// The front end is not traffic: it does not record itself.
	if traces := module.Tracer().Traces(); len(traces) != 1 {
		t.Fatalf("the front end was traced: %+v", traces)
	}
}

func TestModuleObservesRuntimeReports(t *testing.T) {
	module, _ := newTestModule(t)

	ctx, trace, err := module.Tracer().StartTrace(context.Background(), "GET /index.php")
	if err != nil {
		t.Fatal(err)
	}

	module.UpdateStatus(ctx, StateWriting)
	module.UpdateFilename(ctx, "public/index.php")
	module.UpdateIncludedFiles(ctx, 3)
	module.Trace(ctx, "include header.php", KindTemplate).End()
	module.Tracer().Finish(trace)

	if trace.State != StateWriting {
		t.Fatalf("state = %q, want W", trace.State)
	}

	root := trace.Root()
	if root.Attributes["filename"] != "public/index.php" || root.Attributes["included_files"] != 3 {
		t.Fatalf("root span attributes = %+v", root.Attributes)
	}
	if root.SourceText() != "public/index.php" {
		t.Fatalf("root span source = %q", root.SourceText())
	}
	if len(trace.Spans) != 2 || trace.Spans[1].Kind != KindTemplate || !trace.Spans[1].Ended() {
		t.Fatalf("spans = %+v", trace.Spans)
	}
}

func TestModuleTracksLifecycleWork(t *testing.T) {
	module, _ := newTestModule(t)
	failure := errors.New("startup failed")

	err := module.TrackLifecycle(context.Background(), "@startup boot.php", "boot.php", func(ctx context.Context) error {
		StartSpan(ctx, "connect").End()
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the failure returned unchanged", err)
	}

	traces := module.Tracer().Traces()
	if len(traces) != 1 {
		t.Fatalf("traces = %+v", traces)
	}

	// Startup work did not arrive over the network, so it is a background
	// trace rather than a request without a method.
	trace := traces[0]
	if trace.Name != "@startup boot.php" || trace.HTTP != nil || TraceHost(trace) != BackgroundHost {
		t.Fatalf("trace = %+v", trace)
	}
	if trace.State != StateError || trace.Error != failure.Error() {
		t.Fatalf("state = %q, error = %q", trace.State, trace.Error)
	}
	if len(trace.Spans) != 2 || trace.Spans[0].Filename != "boot.php" || trace.Spans[1].Filename != "boot.php" {
		t.Fatalf("spans = %+v", trace.Spans)
	}
}

func TestInstrumentationWithoutATraceDoesNothing(t *testing.T) {
	ctx := WithSpanLine(WithSpanFilename(context.Background(), "index.php"), 12)

	span := StartSpan(ctx, "orphan", KindInternal)
	if span != nil {
		t.Fatalf("span = %+v, want nil without a trace", span)
	}

	// Every method tolerates it, which is what lets a binding instrument
	// unconditionally.
	span.SetAttribute("key", "value")
	span.RecordError(errors.New("ignored"))
	span.End()
}
