package runner

import (
	"fmt"
)

// TraceLevel is how much of its own work the interpreter reports to the
// observers: the `log_level` of the runtime's instrumentation. It decides what
// the runtime publishes and never what the recorder keeps, which is the
// recorder's own minimum duration over the spans this level marks as detail.
//
// A binding, a script calling `start_span` and the host's own middleware are
// outside it. This is the interpreter reporting on itself: a frame, a
// constructor, a static call, an include.
type TraceLevel string

// The levels, lowest first. The zero value is Info: a configuration that names
// no level gets the default, and so does a host that fills in no option.
const (
	// TraceLevelOff publishes no interpreter span at all. A request still
	// carries the spans its bindings and its script recorded, and the error
	// span of a run that failed.
	TraceLevelOff TraceLevel = "off"

	// TraceLevelInfo publishes a span per frame, marked as debug detail so the
	// recorder keeps the slow ones and drops the rest, and collapses the
	// includes into one span at the end of the request naming the files.
	TraceLevelInfo TraceLevel = "info"

	// TraceLevelDebug publishes everything the interpreter can report, marking
	// none of it as detail: a span per include beside the frames, in place of
	// the inventory the info level ends with, and nothing a recorder's minimum
	// duration drops.
	TraceLevelDebug TraceLevel = "debug"
)

// Level answers the level itself, reading the zero value as the default.
func (l TraceLevel) Level() TraceLevel {
	if l == "" {
		return TraceLevelInfo
	}
	return l
}

// Off reports whether the interpreter publishes no span of its own.
func (l TraceLevel) Off() bool { return l.Level() == TraceLevelOff }

// Info reports whether an interpreter span is published as debug detail.
func (l TraceLevel) Info() bool { return l.Level() == TraceLevelInfo }

// Debug reports whether every interpreter span is published unmarked.
func (l TraceLevel) Debug() bool { return l.Level() == TraceLevelDebug }

// Check refuses a level this package does not define. The empty value passes:
// it is what a configuration naming no level decodes to.
func (l TraceLevel) Check() error {
	switch l {
	case "", TraceLevelOff, TraceLevelInfo, TraceLevelDebug:
		return nil
	}
	return fmt.Errorf("runner: trace_level %q is not one of %q, %q or %q", string(l), TraceLevelOff, TraceLevelInfo, TraceLevelDebug)
}
