package runner

import (
	"fmt"
)

// TraceLevel is how much interpreter work the observers see: the runtime's
// `log_level` setting. It decides the runtime's published output. The
// recorder's retained spans stay the recorder's call, through the duration
// floor over the detail spans.
//
// The level never touches a binding, a script calling `start_span` or the
// host's own middleware. This is the interpreter reporting on itself: one frame,
// a constructor, a static call, an include.
type TraceLevel string

// The levels, lowest first. The zero value is Info: the default serves a
// configuration that names no level. It serves a host that fills in no option
// too.
const (
	// TraceLevelOff publishes no interpreter span. A request still
	// holds the spans its bindings and its script recorded, and the error
	// span of a run that failed.
	TraceLevelOff TraceLevel = "off"

	// TraceLevelInfo publishes a span per frame, marked as debug detail so the
	// recorder keeps the slow ones, dropping the rest. It collapses the
	// includes into a single span at the end of the request, naming the files.
	TraceLevelInfo TraceLevel = "info"

	// TraceLevelDebug publishes everything the interpreter can report, marking
	// none of it as detail: a span per include next to the frames, replacing
	// the inventory the `info` level ends with. A recorder's duration floor
	// drops nothing.
	TraceLevelDebug TraceLevel = "debug"
)

// Level answers the level itself, reading the zero value as the default.
func (l TraceLevel) Level() TraceLevel {
	if l == "" {
		return TraceLevelInfo
	}
	return l
}

// Off reports whether the level publishes no interpreter span.
func (l TraceLevel) Off() bool { return l.Level() == TraceLevelOff }

// Info reports whether the level publishes an interpreter span as debug detail.
func (l TraceLevel) Info() bool { return l.Level() == TraceLevelInfo }

// Debug reports whether the level publishes every interpreter span unmarked.
func (l TraceLevel) Debug() bool { return l.Level() == TraceLevelDebug }

// Check refuses a level this package does not define. The empty value passes:
// a configuration naming no level decodes to it.
func (l TraceLevel) Check() error {
	switch l {
	case "", TraceLevelOff, TraceLevelInfo, TraceLevelDebug:
		return nil
	}
	return fmt.Errorf("runner: trace_level %q is not one of %q, %q or %q", string(l), TraceLevelOff, TraceLevelInfo, TraceLevelDebug)
}
