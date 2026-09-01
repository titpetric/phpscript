package telemetry

import (
	"context"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/model"
	"github.com/titpetric/oida/storage"
)

// The recorded data and the recorder both live in oida. These aliases spell
// them the way the rest of phpscript spells its own types, so instrumenting a
// package needs this import alone.
type (
	// Trace is one recorded unit of work: an HTTP request, a startup step or a
	// background job.
	Trace = oida.Trace

	// Span is one timed operation within a trace. Every method tolerates a nil
	// receiver.
	Span = oida.Span

	// Attributes is a set of key/value pairs recorded on a span.
	Attributes = oida.Attributes

	// Kind classifies the work a span measured.
	Kind = oida.Kind

	// State is the scoreboard state of a trace in flight.
	State = oida.State

	// Options configures recording, the middleware and the debug front end.
	Options = oida.Options

	// Tracer records traces and backs the debug front end.
	Tracer = oida.Tracer

	// Sampler decides whether a request is traced.
	Sampler = oida.Sampler

	// Storage retains completed traces.
	Storage = oida.Storage

	// Snapshot is the complete read model of a tracer at one point in time.
	Snapshot = oida.Snapshot

	// Router is the subset of a router needed to mount the debug front end,
	// satisfied by chi.Router, which is what platform.Router is.
	Router = oida.Router
)

// Span kinds. The set is open: an unrecognized value is valid, which is what
// lets PHP pass a plain string.
const (
	KindInternal = oida.KindInternal
	KindHTTP     = oida.KindHTTP
	KindDatabase = oida.KindDatabase
	KindExternal = oida.KindExternal
	KindTemplate = oida.KindTemplate
	KindCache    = oida.KindCache
	KindQueue    = oida.KindQueue
)

// Scoreboard states of a trace in flight. The one-character values follow the
// convention used by servers such as lighttpd.
const (
	StateWaiting    = oida.StateWaiting
	StateStarting   = oida.StateStarting
	StateReading    = oida.StateReading
	StateProcessing = oida.StateProcessing
	StateWriting    = oida.StateWriting
	StateKeepalive  = oida.StateKeepalive
	StateClosing    = oida.StateClosing
	StateError      = oida.StateError
)

const (
	// DefaultPath is the mount path of the debug front end.
	DefaultPath = oida.DefaultPath

	// RequestIDHeader carries the trace identifier on the request and the
	// response.
	RequestIDHeader = oida.RequestIDHeader

	// BackgroundHost is the host label of traces that did not arrive over the
	// network: startup steps, cron ticks, queue consumers. oida stopped
	// re-exporting it from its root package; the constant itself is still the
	// recorded data's, so this reads it from model rather than inventing a
	// second label for the same group.
	BackgroundHost = model.BackgroundHost
)

// Recording and configuration failures. Every configuration failure wraps
// ErrInvalidOptions.
var (
	ErrNilRouter         = oida.ErrNilRouter
	ErrInvalidOptions    = oida.ErrInvalidOptions
	ErrInvalidPath       = oida.ErrInvalidPath
	ErrInvalidSampleRate = oida.ErrInvalidSampleRate
	ErrTraceNotFound     = oida.ErrTraceNotFound
	ErrDisabled          = oida.ErrDisabled
)

// NewOptions returns the default options for a service of that name, which the
// front end displays and every trace records. oida applies the OIDA_*
// environment to options built this way, so a deployment still configures
// itself; options built as a literal read none.
func NewOptions(serviceName string) Options {
	return oida.NewOptions(serviceName)
}

// New returns a tracer configured with opts. It is the only constructor: oida
// no longer keeps a process wide tracer, so a caller holds the one it built and
// hands it to whatever records into it.
func New(opts Options) (*Tracer, error) {
	return oida.New(opts)
}

// Start records a span in the trace carried by ctx and returns a context
// carrying it, so spans started from that context nest below this one.
func Start(ctx context.Context, name string, kind ...Kind) (context.Context, *Span) {
	ctx, span := oida.Start(ctx, name, kind...)
	return ctx, withSource(ctx, span)
}

// StartSpan records a span without deriving a context. Use it for leaf spans
// that will not nest. The source location carried by ctx, when a PHP frame put
// one there, is recorded on the span.
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span {
	return withSource(ctx, oida.StartSpan(ctx, name, kind...))
}

// Do runs fn inside a span, records the returned error on it and ends it. The
// error is returned unchanged.
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error {
	return oida.Do(ctx, name, fn, kind...)
}

// WithTrace returns a context carrying the trace. Spans started from it, or
// from any context derived from it, are recorded on that trace.
func WithTrace(ctx context.Context, t *Trace) context.Context {
	return oida.WithTrace(ctx, t)
}

// TraceFromContext returns the trace in ctx, or nil.
func TraceFromContext(ctx context.Context) *Trace {
	return oida.TraceFromContext(ctx)
}

// SpanFromContext returns the innermost span in ctx, or nil.
func SpanFromContext(ctx context.Context) *Span {
	return oida.SpanFromContext(ctx)
}

// TraceID returns the identifier of the trace in ctx, or an empty string. It is
// the value of the Request-Id header for HTTP traces, which makes it the
// cheapest correlation key for logs.
func TraceID(ctx context.Context) string {
	return oida.TraceID(ctx)
}

// TraceHost returns the host a trace belongs to. Background traces did not
// arrive over the network and have no host, so they group under
// BackgroundHost. oida dropped the function along with the rest of its view
// model; the grouping is still what the server's own reporting reads by, so it
// is stated here rather than at each caller.
func TraceHost(trace Trace) string {
	if trace.HTTP == nil || trace.HTTP.Host == "" {
		return BackgroundHost
	}
	return trace.HTTP.Host
}

// NewStorageDisk returns storage retaining at most limit traces as JSON
// documents, so they survive a restart.
//
// This is the one place phpscript names an oida sub-package. oida selects a
// driver from OIDA_STORAGE_DRIVER and the OIDA_STORAGE_* variables, and the
// telemetry block in a site's configuration is what phpscript configures a
// site by; reaching the constructor directly is what keeps `driver: disk`
// meaning what it says instead of moving that choice into the environment.
func NewStorageDisk(limit int, paths ...string) (Storage, error) {
	return storage.NewDiskStorage(limit, paths...)
}

// Mount registers the debug front end on r under Options.Path, serving the
// traces of that tracer. oida takes the tracer itself now, there being no
// process wide one to resolve from options.
func Mount(r Router, tracer *Tracer) error {
	return oida.Mount(r, tracer)
}
