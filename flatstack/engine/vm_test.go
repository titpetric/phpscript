package engine

import (
	"testing"

	"github.com/titpetric/phpscript/parser"
)

// refTestHost is the part of the host contract these tests exercise: the
// frame handle a real host holds, the snapshot-before-call and write-back-
// after ordering of its slow path, plus the calls themselves. Every other
// method is left to the embedded nil interface, so a test program that
// reaches one fails loudly rather than silently.
type refTestHost struct {
	Host
	frame  FrameLocals
	locals map[string]any
	echoed []any
	// calls names what each call does, keyed by function name: it either
	// writes the by-reference setter it was handed, or writes a local through
	// the snapshot the way a host binding that touches the scope does.
	calls map[string]func(h *refTestHost, args []any)
}

func (h *refTestHost) SetGlobal(string, any) bool { return false }

func (h *refTestHost) BindFrame(frame FrameLocals) { h.frame = frame }

func (h *refTestHost) TakeFrame() FrameLocals { return h.frame }

// Call takes the slow path a scope-reading binding takes: snapshot before the
// body runs, write the snapshot back after. That ordering is what the
// by-reference mark tests pin.
func (h *refTestHost) Call(name, fallback string, args []any) (any, error) {
	h.locals = h.frame.Snapshot()
	if fn, ok := h.calls[name]; ok {
		fn(h, args)
	}
	h.frame.WriteBack(h.locals)
	return int64(0), nil
}

func (h *refTestHost) Echo(value any) error {
	h.echoed = append(h.echoed, value)
	return nil
}

func (h *refTestHost) InvokeCallable(callable any) error {
	if fn, ok := callable.(func()); ok {
		fn()
	}
	return nil
}

func setRef(value any) func(*refTestHost, []any) {
	return func(_ *refTestHost, args []any) {
		args[len(args)-1].(func(any))(value)
	}
}

func setLocal(name string, value any) func(*refTestHost, []any) {
	return func(h *refTestHost, _ []any) { h.locals[name] = value }
}

func runRefProgram(t *testing.T, source string, calls map[string]func(*refTestHost, []any)) []any {
	t.Helper()
	ast, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	program, err := Compile(ast)
	if err != nil {
		t.Fatal(err)
	}
	host := &refTestHost{calls: calls}
	if err := Run(program, host); err != nil {
		t.Fatal(err)
	}
	return host.echoed
}

// The locals snapshot the host was handed predates the call, so it still holds
// the value the out parameter replaced. Writing it back would undo the write,
// which is what made a reused $m keep the first call's matches.
func TestVMRefSetterSurvivesLocalsWriteBack(t *testing.T) {
	echoed := runRefProgram(t, `<?php
		preg_match_all("/a/", "aa", $m);
		preg_match_all("/b/", "bbb", $m);
		echo $m;
	`, map[string]func(*refTestHost, []any){
		"preg_match_all": func(h *refTestHost, args []any) {
			// Two calls in sequence, so the second one writes a slot the
			// snapshot already carries a value for.
			if _, ok := h.locals["m"]; ok {
				setRef("second")(h, args)
				return
			}
			setRef("first")(h, args)
		},
	})

	if len(echoed) != 1 || echoed[0] != "second" {
		t.Errorf("echoed = %v, want [second]", echoed)
	}
}

// An uninitialised target is the case that always worked: it is not in the
// snapshot, so nothing overwrites it. It has to keep working.
func TestVMRefSetterWritesFreshLocal(t *testing.T) {
	echoed := runRefProgram(t, `<?php
		preg_match_all("/a/", "aa", $m);
		echo $m;
	`, map[string]func(*refTestHost, []any){
		"preg_match_all": setRef("matches"),
	})

	if len(echoed) != 1 || echoed[0] != "matches" {
		t.Errorf("echoed = %v, want [matches]", echoed)
	}
}

// The marks cover one call. A later call that changes the same variable
// through the scope, with no setter involved, still takes effect.
func TestVMRefMarksDoNotOutliveTheCall(t *testing.T) {
	echoed := runRefProgram(t, `<?php
		preg_match_all("/a/", "aa", $m);
		clobber();
		echo $m;
	`, map[string]func(*refTestHost, []any){
		"preg_match_all": setRef("matches"),
		"clobber":        setLocal("m", "clobbered"),
	})

	if len(echoed) != 1 || echoed[0] != "clobbered" {
		t.Errorf("echoed = %v, want [clobbered]", echoed)
	}
}

// A by-reference call inside a user function marks that function's frame. The
// caller's variable of the same name is a different slot in a different frame
// and must not be shielded from its own write-back.
func TestVMRefMarksArePerFrame(t *testing.T) {
	echoed := runRefProgram(t, `<?php
		function inner() {
			preg_match_all("/a/", "aa", $m);
			return $m;
		}
		$m = "outer";
		echo inner();
		clobber();
		echo $m;
	`, map[string]func(*refTestHost, []any){
		"preg_match_all": setRef("inner matches"),
		"clobber":        setLocal("m", "clobbered"),
	})

	want := []any{"inner matches", "clobbered"}
	if len(echoed) != len(want) || echoed[0] != want[0] || echoed[1] != want[1] {
		t.Errorf("echoed = %v, want %v", echoed, want)
	}
}

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
