package engine

import (
	"testing"
)

// release has to zero the whole backing array, not just the live prefix. The
// pool holds the buffers for the life of the process, so a value left above
// the high-water mark of a later, smaller program would stay reachable through
// the pool and never be collected.
func TestExecStateReleaseClearsToCapacity(t *testing.T) {
	st := &execState{
		stack:       make([]any, 0, 8),
		locals:      make([]any, 4),
		initialized: make([]bool, 4),
		iterators:   make([]*iteratorState, 2),
		handlers:    make([]errorHandler, 1),
		callFrames:  make([]callFrame, 1),
	}

	// Fill every slot, including the part of the stack above its length.
	st.stack = st.stack[:cap(st.stack)]
	for i := range st.stack {
		st.stack[i] = "retained"
	}
	for i := range st.locals {
		st.locals[i] = "retained"
		st.initialized[i] = true
	}
	st.iterators[0] = &iteratorState{source: "retained"}
	st.callFrames[0] = callFrame{locals: []any{"retained"}}

	// A program that used two stack slots and returned hands back a short
	// slice; the six slots above it still hold values.
	st.stack = st.stack[:2]
	st.release()

	for i, slot := range st.stack[:cap(st.stack)] {
		if slot != nil {
			t.Errorf("stack[%d] = %v, want nil", i, slot)
		}
	}
	for i, slot := range st.locals[:cap(st.locals)] {
		if slot != nil {
			t.Errorf("locals[%d] = %v, want nil", i, slot)
		}
	}
	for i, slot := range st.initialized[:cap(st.initialized)] {
		if slot {
			t.Errorf("initialized[%d] = true, want false", i)
		}
	}
	for i, it := range st.iterators[:cap(st.iterators)] {
		if it != nil {
			t.Errorf("iterators[%d] = %v, want nil", i, it)
		}
	}
	for i, frame := range st.callFrames[:cap(st.callFrames)] {
		if frame.locals != nil {
			t.Errorf("callFrames[%d].locals = %v, want nil", i, frame.locals)
		}
	}
	if len(st.stack) != 0 {
		t.Errorf("stack length = %d, want 0", len(st.stack))
	}
}

// An empty state is what the pool hands out for the first program with no
// locals, and release must not panic on it.
func TestExecStateReleaseEmpty(t *testing.T) {
	st := &execState{}
	st.release()
}
