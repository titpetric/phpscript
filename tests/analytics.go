package tests

// This file is the reference subject for *dispatch cost*, the way bindings.go is
// the reference subject for return shapes. It answers one question, "what does
// it cost to get from PHP into Go and back?", by registering the same work
// under several names, each of which the runtime reaches by a different path.
//
// The work is deliberately trivial and allocation-free: a fixed-size ring
// buffer that records a request and overwrites the oldest entry. That is the
// analytics use case a script serves when its only job is to call into the
// host, and it means a benchmark measures the crossing rather than the callee.
//
// The names, and the path each one exercises:
//
//	Analytics\record        namespaced, so the transpiler emits __func(...)
//	analytics_record        global, so the transpiler emits a bare identifier
//	analytics_record_fast   func(...any) (any, error), the invokeFast fast case
//	analytics_record_ctx    leading context.Context, so a context is derived
//
// All four write to the same ring through the same underlying func value, so a
// difference between two of them is dispatch and nothing else.

import (
	"context"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// AnalyticsEntry is one recorded request. The fields are fixed-size and hold no
// pointers the ring does not already own, so recording is a store into
// preallocated storage rather than an allocation.
type AnalyticsEntry struct {
	Route  string
	Status int64
	Micros int64
}

// AnalyticsRing is a fixed-size buffer that overwrites its oldest entry. It is
// the shape a service uses to keep the last N requests in memory without
// growing: storage is allocated once and Record never allocates.
//
// The lock is a plain mutex rather than an atomic index. An atomic index is
// cheaper in isolation, but two goroutines can then be handed slots i and
// i+size concurrently and write the same memory, which is a race whether or not
// a benchmark happens to catch it. The mutex is uncontended in every
// single-goroutine benchmark and identical in every cell of the matrix, so it
// cancels out of every comparison between cells; it is not what the numbers in
// docs/php-go-calls.md are measuring.
type AnalyticsRing struct {
	mu      sync.Mutex
	entries []AnalyticsEntry
	mask    uint64
	n       uint64
}

// DefaultAnalyticsRing is the ring the registered bindings write to. One ring
// per process keeps a fixture's counts comparable across runs of the same
// fixture, which is what lets analytics_record.phpt assert relative counts.
var DefaultAnalyticsRing = NewAnalyticsRing(1024)

// NewAnalyticsRing returns a ring holding size entries. Size is rounded up to a
// power of two so the slot for a sequence number is a mask rather than a
// division; a size below one is treated as one.
func NewAnalyticsRing(size int) *AnalyticsRing {
	if size < 1 {
		size = 1
	}
	rounded := 1
	for rounded < size {
		rounded <<= 1
	}
	return &AnalyticsRing{
		entries: make([]AnalyticsEntry, rounded),
		mask:    uint64(rounded - 1),
	}
}

// Record stores one entry, overwriting the oldest when the ring is full. It
// allocates nothing: the slot already exists and the three fields are assigned
// into it.
func (r *AnalyticsRing) Record(route string, status, micros int64) {
	r.mu.Lock()
	r.entries[r.n&r.mask] = AnalyticsEntry{Route: route, Status: status, Micros: micros}
	r.n++
	r.mu.Unlock()
}

// Count returns how many entries have been recorded over the ring's lifetime,
// which keeps counting past the point where the ring starts overwriting.
func (r *AnalyticsRing) Count() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(r.n)
}

// Last returns the entry recorded most recently, or nil when none has been.
// The pointer addresses the ring's own storage, so returning it boxes a pointer
// without copying or allocating an entry; a caller that holds it past the next
// wrap sees the newer entry, which is the trade a ring makes.
func (r *AnalyticsRing) Last() *AnalyticsEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.n == 0 {
		return nil
	}
	return &r.entries[(r.n-1)&r.mask]
}

// Snapshot returns the entries still held, oldest first. It copies, because the
// ring overwrites underneath a caller that does not.
func (r *AnalyticsRing) Snapshot() []AnalyticsEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	size := uint64(len(r.entries))
	held := r.n
	if held > size {
		held = size
	}
	out := make([]AnalyticsEntry, 0, held)
	for i := r.n - held; i < r.n; i++ {
		out = append(out, r.entries[i&r.mask])
	}
	return out
}

// analyticsRegistrar is the slice of *runner.Runtime the analytics bindings
// need. Constructors are separate from functions because only RegisterFunc
// invalidates the runtime's compiled-expression caches.
type analyticsRegistrar interface {
	RegisterFunc(name string, fn any)
	RegisterConstructor(name string, ctor any)
}

// RegisterAnalytics installs the analytics bindings on rt, all writing to
// DefaultAnalyticsRing.
func RegisterAnalytics(rt *runner.Runtime) {
	registerAnalytics(rt, DefaultAnalyticsRing)
}

// AnalyticsFuncs returns the raw (unregistered, unadapted) analytics bindings
// keyed by the PHP name they are registered under, so a benchmark can call one
// directly and measure the reflection path without a Runtime.
func AnalyticsFuncs(ring *AnalyticsRing) map[string]any {
	funcs := map[string]any{}
	registerAnalytics(&analyticsCollector{funcs: funcs}, ring)
	return funcs
}

// analyticsCollector captures registrations without a Runtime, the way
// collector does for bindings.go, so one set of definitions serves the VM
// benchmarks and the direct-call ones.
type analyticsCollector struct {
	funcs map[string]any
}

func (c *analyticsCollector) RegisterFunc(name string, fn any) { c.funcs[name] = fn }

func (c *analyticsCollector) RegisterConstructor(name string, _ any) {}

func registerAnalytics(rt analyticsRegistrar, ring *AnalyticsRing) {
	// native is the signature a binding is naturally written with: concrete
	// parameters, no context, no error. It goes through buildArgs and
	// reflect.Value.Call.
	native := func(route string, status, micros int64) {
		ring.Record(route, status, micros)
	}

	// Analytics\record records one request. $route is the path served, $status the
	// HTTP status, and $micros how long it took in microseconds; the oldest entry is
	// overwritten once the buffer is full.
	rt.RegisterFunc("Analytics\\record", native)

	// analytics_record is Analytics\record under a global name. The two are the same
	// function writing to the same buffer, registered twice so a benchmark can
	// compare what a namespaced call costs against a global one.
	rt.RegisterFunc("analytics_record", native)

	// analytics_record_fast is analytics_record written in the uniform shape the
	// runtime can call without reflection. It is registered to measure what that
	// shape is worth, not because a binding should normally be written this way.
	rt.RegisterFunc("analytics_record_fast", func(args ...any) (any, error) {
		route, _ := argString(args, 0)
		status, _ := argInt(args, 1)
		micros, _ := argInt(args, 2)
		ring.Record(route, status, micros)
		return nil, nil
	})

	// analytics_record_ctx is analytics_record with a leading context, so a
	// benchmark can price the context the runtime derives for a binding that asks
	// for one.
	rt.RegisterFunc("analytics_record_ctx", func(_ context.Context, route string, status, micros int64) {
		ring.Record(route, status, micros)
	})

	// Analytics\count returns how many requests have been recorded, which keeps
	// counting past the point where the buffer starts overwriting.
	rt.RegisterFunc("Analytics\\count", func() int64 { return ring.Count() })

	// Analytics\last returns the request recorded most recently, or null when none
	// has been. Read its $route, $status and $micros with property access.
	rt.RegisterFunc("Analytics\\last", func() any {
		entry := ring.Last()
		if entry == nil {
			return nil
		}
		return entry
	})

	// Analytics\Buffer is the recording buffer as an object, so a script can hold
	// one and call $buffer->record() on it. It shares the buffer the
	// Analytics\record function writes to.
	rt.RegisterConstructor("Analytics\\Buffer", func() *AnalyticsRing { return ring })
}

// argString reads args[i] as a string, reporting whether it was one.
func argString(args []any, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	s, ok := args[i].(string)
	return s, ok
}

// argInt reads args[i] as a PHP int, reporting whether it was one. PHP ints
// arrive as int64; a float is accepted because a literal division produces one.
func argInt(args []any, i int) (int64, bool) {
	if i >= len(args) {
		return 0, false
	}
	switch v := args[i].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	}
	return 0, false
}
