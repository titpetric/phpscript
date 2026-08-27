package engine

import (
	"testing"

	"github.com/titpetric/phpscript/parser"
)

// refTestHost is the part of the host contract these tests exercise: the
// locals snapshot a real host takes around a call, plus the calls themselves.
// Every other method is left to the embedded nil interface, so a test program
// that reaches one fails loudly rather than silently.
type refTestHost struct {
	Host
	locals map[string]any
	echoed []any
	// calls names what each call does, keyed by function name: it either
	// writes the by-reference setter it was handed, or writes a local through
	// the snapshot the way a host binding that touches the scope does.
	calls map[string]func(h *refTestHost, args []any)
}

func (h *refTestHost) SetGlobal(string, any) bool { return false }

func (h *refTestHost) BindLocals(vars map[string]any) { h.locals = vars }

func (h *refTestHost) TakeLocals() map[string]any { return h.locals }

func (h *refTestHost) Call(name, fallback string, args []any) (any, error) {
	if fn, ok := h.calls[name]; ok {
		fn(h, args)
	}
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
