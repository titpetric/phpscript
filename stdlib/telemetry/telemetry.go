package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// Span represents a single timed operation span.
type Span struct {
	ULID       string `json:"ulid"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	StartNs    int64  `json:"start_ns"`
	DurationNs int64  `json:"duration_ns"`
}

// RingBuffer stores the last N request spans thread-safely with 0-alloc pool recycling.
type RingBuffer struct {
	mu       sync.RWMutex
	capacity int
	spans    []*Span
	head     int
	count    int
	spanPool sync.Pool
}

var globalTracker = NewRingBuffer(1000)

// Global returns the package-level telemetry RingBuffer.
func Global() *RingBuffer {
	return globalTracker
}

// NewRingBuffer creates a new RingBuffer bounded to capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 1000
	}
	rb := &RingBuffer{
		capacity: capacity,
		spans:    make([]*Span, capacity),
	}
	rb.spanPool.New = func() any {
		return &Span{}
	}
	return rb
}

// AcquireSpan gets a Span from the pool or allocates a new one.
func (rb *RingBuffer) AcquireSpan(ulid, name, category string) *Span {
	s := rb.spanPool.Get().(*Span)
	s.ULID = ulid
	s.Name = name
	s.Category = category
	s.StartNs = time.Now().UnixNano()
	s.DurationNs = 0
	return s
}

// Finish calculates duration and records span into the ring buffer, recycling replaced spans.
func (rb *RingBuffer) Finish(s *Span) {
	if s == nil {
		return
	}
	s.DurationNs = time.Now().UnixNano() - s.StartNs

	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.spans[rb.head] != nil {
		rb.spanPool.Put(rb.spans[rb.head])
	}

	rb.spans[rb.head] = s
	rb.head = (rb.head + 1) % rb.capacity
	if rb.count < rb.capacity {
		rb.count++
	}
}

// Spans returns a slice of value copies of all current recorded spans in chronological order.
func (rb *RingBuffer) Spans() []Span {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	var raw []*Span
	if rb.count < rb.capacity {
		raw = rb.spans[:rb.count]
	} else {
		raw = append(raw, rb.spans[rb.head:]...)
		raw = append(raw, rb.spans[:rb.head]...)
	}

	out := make([]Span, 0, len(raw))
	for _, s := range raw {
		if s != nil {
			out = append(out, *s)
		}
	}
	return out
}

// Clear resets the ring buffer and recycles spans into the pool.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for i := range rb.spans {
		if rb.spans[i] != nil {
			rb.spanPool.Put(rb.spans[i])
			rb.spans[i] = nil
		}
	}
	rb.head = 0
	rb.count = 0
}

// Register registers the PHP bridge for PS\Telemetry constructor on Runtime.
func Register(rt *runner.Runtime) {
	rt.RegisterConstructor("PS\\Telemetry", NewPHPBridge)
	rt.RegisterConstructor("Telemetry", NewPHPBridge)
}

// NewPHPBridge constructor bridge for PHP `new PS\Telemetry($name, ...)`
func NewPHPBridge(ctx context.Context, name ...string) (*PHPBridge, error) {
	spanName := "span"
	if len(name) > 0 && name[0] != "" {
		spanName = name[0]
	}
	category := "php"
	if len(name) > 1 && name[1] != "" {
		category = name[1]
	}

	ulid := "request-ulid"
	span := globalTracker.AcquireSpan(ulid, spanName, category)
	return &PHPBridge{span: span}, nil
}

// PHPBridge provides methods exposed to PHP scripts.
type PHPBridge struct {
	span *Span
}

// Close finishes and records the telemetry span.
func (b *PHPBridge) Close() bool {
	if b.span != nil {
		globalTracker.Finish(b.span)
		b.span = nil
	}
	return true
}

// End finishes and records the telemetry span.
func (b *PHPBridge) End() bool {
	return b.Close()
}

// SpansAsModelArray returns current recorded spans as a *model.Array for PHP inspection.
func SpansAsModelArray() *model.Array {
	spans := globalTracker.Spans()
	arr := model.NewArray()
	for i, s := range spans {
		m := model.NewArray()
		m.Set("ulid", s.ULID)
		m.Set("name", s.Name)
		m.Set("category", s.Category)
		m.Set("start_ns", s.StartNs)
		m.Set("duration_ns", s.DurationNs)
		arr.Set(int64(i), m)
	}
	return arr
}
