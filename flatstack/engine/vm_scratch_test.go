package engine

import "testing"

// release has to zero the whole backing array, not just the live prefix. The
// pool holds the buffers for the life of the process, so a value left above
// the high-water mark of a later, smaller program would stay reachable through
// the pool and never be collected.
func TestVMScratchReleaseClearsToCapacity(t *testing.T) {
	scratch := &vmScratch{
		stack:       make([]any, 0, 8),
		locals:      make([]any, 4),
		initialized: make([]bool, 4),
	}

	// Fill every slot, including the part of the stack above its length.
	stack := scratch.stack[:cap(scratch.stack)]
	for i := range stack {
		stack[i] = "retained"
	}
	for i := range scratch.locals {
		scratch.locals[i] = "retained"
		scratch.initialized[i] = true
	}

	// A program that used two stack slots and returned hands back a short
	// slice; the six slots above it still hold values.
	scratch.release(stack[:2])

	for i, slot := range scratch.stack[:cap(scratch.stack)] {
		if slot != nil {
			t.Errorf("stack[%d] = %v, want nil", i, slot)
		}
	}
	for i, slot := range scratch.locals[:cap(scratch.locals)] {
		if slot != nil {
			t.Errorf("locals[%d] = %v, want nil", i, slot)
		}
	}
	for i, slot := range scratch.initialized[:cap(scratch.initialized)] {
		if slot {
			t.Errorf("initialized[%d] = true, want false", i)
		}
	}
	if len(scratch.stack) != 0 {
		t.Errorf("stack length = %d, want 0", len(scratch.stack))
	}
}

// A nil scratch is what the pool hands out for the first program with no
// locals, and release must not panic on it.
func TestVMScratchReleaseEmpty(t *testing.T) {
	scratch := &vmScratch{}
	scratch.release(nil)
}
