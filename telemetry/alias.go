package telemetry

import (
	"context"
	"net/http"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
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

	// Recorder is the substitutable subset of Tracer.
	Recorder = oida.Recorder

	// Sampler decides whether a request is traced.
	Sampler = oida.Sampler

	// Storage retains completed traces.
	Storage = oida.Storage

	// HTTPInfo describes the request a trace was created for.
	HTTPInfo = oida.HTTPInfo

	// Memory describes current process memory and GC pressure.
	Memory = oida.Memory

	// MemoryUse holds the allocation deltas observed while a trace ran.
	MemoryUse = oida.MemoryUse

	// PoolEstimate is a heuristic concurrency estimate.
	PoolEstimate = oida.PoolEstimate

	// StateDuration is the lifetime trace time observed in one state.
	StateDuration = oida.StateDuration

	// Statistic aggregates one group of traces in the rolling window.
	Statistic = oida.Statistic

	// HostStat aggregates the traffic of one host.
	HostStat = oida.HostStat

	// Stats contains the most frequent trace groups in the rolling window.
	Stats = oida.Stats

	// Snapshot is the complete read model of a tracer at one point in time.
	Snapshot = oida.Snapshot
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
	// network: startup steps, cron ticks, queue consumers.
	BackgroundHost = oida.BackgroundHost
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

// NewOptions returns the default options.
func NewOptions() Options {
	return oida.NewOptions()
}

// New returns a tracer configured with opts. Prefer it over Configure in
// libraries and tests: it does not touch process wide state.
func New(opts Options) (*Tracer, error) {
	return oida.New(opts)
}

// Configure replaces the process wide tracer with one built from opts and
// returns it.
func Configure(opts Options) (*Tracer, error) {
	return oida.Configure(opts)
}

// Default returns the process wide tracer, creating it on first use.
func Default() *Tracer {
	return oida.Default()
}

// Resolve returns the tracer the options point at: the explicit one when set,
// the process default otherwise.
func Resolve(opts Options) (*Tracer, error) {
	return oida.Resolve(opts)
}

// TracingMiddleware returns middleware recording every sampled request into the
// tracer resolved from opts.
func TracingMiddleware(opts Options) func(http.Handler) http.Handler {
	return oida.TracingMiddleware(opts)
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

// TraceHost returns the host a trace belongs to. Background traces have none,
// so they group under BackgroundHost.
func TraceHost(trace Trace) string {
	return oida.TraceHost(trace)
}

// ValidID reports whether id looks like a recorded trace identifier. It keeps
// hostile input out of lookups and out of rendered links.
func ValidID(id string) bool {
	return oida.ValidID(id)
}

// NewStorageMemory returns in-memory storage retaining size traces.
func NewStorageMemory(size int) Storage {
	return oida.NewStorageMemory(size)
}

// NewStorageDisk returns storage retaining at most limit traces as JSON
// documents, so they survive a restart.
func NewStorageDisk(limit int, paths ...string) (Storage, error) {
	return oida.NewStorageDisk(limit, paths...)
}

// NewRateSampler returns a sampler tracing the given fraction of requests.
func NewRateSampler(rate float64) Sampler {
	return oida.NewRateSampler(rate)
}

// Router is the subset of a router needed to mount the debug front end. It is
// satisfied by chi.Router, which is what platform.Router is.
type Router = frontend.Router

// Mount registers the debug front end on r under Options.Path, wired to the
// tracer resolved from opts.
func Mount(r Router, opts Options) error {
	return frontend.Mount(r, opts)
}

// Handler returns the debug front end handler for the tracer resolved from
// opts.
func Handler(opts Options) http.Handler {
	return frontend.Handler(opts)
}

// HandlerFor returns the debug front end handler of one tracer.
func HandlerFor(tracer *Tracer) http.Handler {
	return frontend.HandlerFor(tracer)
}
