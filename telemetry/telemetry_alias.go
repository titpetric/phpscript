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

	// Sampler decides which requests a tracer records.
	Sampler = oida.Sampler

	// Storage retains completed traces.
	Storage = oida.Storage

	// Snapshot is the complete read model of a tracer at one instant.
	Snapshot = oida.Snapshot

	// Router is the subset of a router needed to mount the debug front end,
	// satisfied by chi.Router, the interface platform.Router names.
	Router = oida.Router
)

// Span kinds. The set is open: an unrecognized value is valid, as
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

// The names a host and a script agree on: where the platform mounts the front
// end, what carries a trace identifier, and the label a trace with no request
// behind it takes.
const (
	// DefaultPath is the mount path of the debug front end.
	DefaultPath = oida.DefaultPath

	// RequestIDHeader carries the trace identifier on the request and the
	// response.
	RequestIDHeader = oida.RequestIDHeader

	// BackgroundHost is the host label of traces that did not arrive over the
	// network: startup steps, cron ticks, queue consumers. oida stopped
	// re-exporting it from its root package; the constant itself is still the
	// recorded data's, so this reads it from model and invents no
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

// NewOptions returns the default options for a service, under OIDA_*.
func NewOptions(serviceName string) Options {
	return oida.NewOptions(serviceName)
}

// New returns a tracer configured with opts, which its caller holds.
func New(opts Options) (*Tracer, error) {
	return oida.New(opts)
}

// Start records a span in the trace carried by ctx and returns a context
// carrying it, so spans started from that context nest below this one.
func Start(ctx context.Context, name string, kind ...Kind) (context.Context, *Span) {
	ctx, span := oida.Start(ctx, name, kind...)
	return ctx, withSource(ctx, span)
}

// StartSpan records a leaf span, deriving no context for one to nest under.
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span {
	return withSource(ctx, oida.StartSpan(ctx, name, kind...))
}

// Do runs fn inside a span, recording the error it returns.
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error {
	return oida.Do(ctx, name, fn, kind...)
}

// WithTrace returns a context whose spans land on the trace.
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

// TraceID returns the identifier of the trace in ctx.
func TraceID(ctx context.Context) string {
	return oida.TraceID(ctx)
}

// TraceHost returns the host a trace belongs to, and BackgroundHost for a
// trace that arrived over no network.
func TraceHost(trace Trace) string {
	if trace.HTTP == nil || trace.HTTP.Host == "" {
		return BackgroundHost
	}
	return trace.HTTP.Host
}

// NewStorageDisk returns storage retaining at most limit traces as JSON.
func NewStorageDisk(limit int, paths ...string) (Storage, error) {
	return storage.NewDiskStorage(limit, paths...)
}

// Mount registers the debug front end on r under Options.Path.
func Mount(r Router, tracer *Tracer) error {
	return oida.Mount(r, tracer)
}
