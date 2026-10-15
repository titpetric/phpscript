package engine

import (
	"testing"

	"github.com/titpetric/phpscript/parser"
)

// refTestHost is the part of the host contract these tests exercise: the
// frame handle a real host holds, the snapshot-before-call and write-back-
// after ordering of its slow path, plus the calls themselves. Every other
// method is left to the embedded nil interface, so a test program that
// reaches one fails loudly, and never silently.
type refTestHost struct {
	Host
	frame  FrameLocals
	locals map[string]any
	echoed []any
	// calls names what each call does, keyed by function name: it either
	// writes the by-reference setter it holds, or writes a local through
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
	if err := Run(host, program); err != nil {
		t.Fatal(err)
	}
	return host.echoed
}

// The locals snapshot the host holds predates the call, so it still carries
// the value the out parameter replaces. Writing it back would undo the write
// and leave a reused $m holding the first call's matches.
func TestVMRefSetterSurvivesLocalsWriteBack(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

	// A program that used two stack slots and returned releases a short
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

// An empty state is what the pool returns for the first program with no
// locals, and release must not panic on it.
func TestExecStateReleaseEmpty(t *testing.T) {
	st := &execState{}
	st.release()
}

// memoryTestHost is a Host that also accounts memory. The capability is found
// by type assertion, so implementing the four methods is the whole opt-in.
type memoryTestHost struct {
	semanticHost
	interval int
	pushed   int
	popped   int
	checks   int
	walk     func(yield func(any))
	seen     []any
}

func (h *memoryTestHost) MemoryCheckInterval() int { return h.interval }

func (h *memoryTestHost) PushLiveWalker(walk func(yield func(any))) {
	h.pushed++
	h.walk = walk
}

func (h *memoryTestHost) PopLiveWalker() { h.popped++ }

// CheckMemory walks the live values while the run holds them, which is the
// window the walker is valid in: the state goes back to the pool once Run
// returns.
func (h *memoryTestHost) CheckMemory() error {
	h.checks++
	if h.walk != nil {
		h.walk(func(value any) { h.seen = append(h.seen, value) })
	}
	return nil
}

// TestMemoryHost covers the accounting hook the VM calls on a host that limits
// memory. The interval is read once per run and decides how often CheckMemory
// is called; the walker is registered for the length of the run and removed
// on the way out, so the pooled state it closes over is never enumerated
// past its release. The walk reaches the frame's initialised locals,
// where a script's values are.
func TestMemoryHost(t *testing.T) {
	t.Parallel()
	host := &memoryTestHost{interval: 8}

	runHost(t, host, `<?php $marker = "retained"; for ($i = 0; $i < 60; $i = $i + 1) { $marker = "retained"; }`)

	if host.pushed != 1 || host.popped != 1 {
		t.Errorf("walker pushed %d times and popped %d, want 1 and 1", host.pushed, host.popped)
	}
	if host.checks == 0 {
		t.Fatal("CheckMemory was never called, want one call per interval of instructions")
	}

	found := false
	for _, value := range host.seen {
		if value == "retained" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("the walk visited %d values and none was the script's local", len(host.seen))
	}

	// A zero interval turns the checks off. The walker is still registered,
	// so a host that only answers usage queries keeps seeing VM state.
	quiet := &memoryTestHost{}
	runHost(t, quiet, `<?php $marker = "retained"; for ($i = 0; $i < 60; $i = $i + 1) { $marker = "retained"; }`)
	if quiet.checks != 0 {
		t.Errorf("CheckMemory was called %d times at a zero interval, want 0", quiet.checks)
	}
	if quiet.pushed != 1 || quiet.popped != 1 {
		t.Errorf("walker pushed %d times and popped %d at a zero interval, want 1 and 1", quiet.pushed, quiet.popped)
	}
}

// deadlineTestHost is a Host that also answers for the execution deadline.
type deadlineTestHost struct {
	semanticHost
	checks int
}

func (h *deadlineTestHost) CheckDeadline() error {
	h.checks++
	return nil
}

// TestDeadlineHost covers the deadline hook. The interval is a constant in
// the engine and not something the host settles once per run, because a
// script can call set_time_limit after the run started. A run that answers
// nil carries on to the end, so the cost of a script with no limit and no
// client is the call itself.
func TestDeadlineHost(t *testing.T) {
	t.Parallel()
	long := &deadlineTestHost{}
	runHost(t, long, `<?php $total = 0; for ($i = 0; $i < 200; $i = $i + 1) { $total = $total + $i; } echo $total;`)

	if long.checks == 0 {
		t.Error("CheckDeadline was never called over a loop of 200 iterations")
	}
	if len(long.echoed) != 1 || long.echoed[0] != int64(19900) {
		t.Errorf("echoed = %v, want the loop to have run to the end", long.echoed)
	}

	// A program shorter than the check interval is never asked, so a small
	// script pays nothing for the capability.
	short := &deadlineTestHost{}
	runHost(t, short, `<?php echo "hi";`)
	if short.checks != 0 {
		t.Errorf("CheckDeadline was called %d times for a two-instruction program, want 0", short.checks)
	}
}
