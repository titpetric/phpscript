package core

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestRegisterLog covers where error_log() puts a message, and what it answers.
//
// php's error_log writes to the SAPI's log and the SAPI decides what that is.
// Here a host that installed a handler with OnError receives it there and the
// message reaches the application and no process error stream; the stream is
// the fallback for a run with nothing listening, because an error log that
// discards is worse than none. The call answers true either way.
func TestRegisterLog(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})

	var recorded []string
	rt.OnError(func(err error) { recorded = append(recorded, err.Error()) })
	registerLog(rt)

	if !rt.HasErrorHandler() {
		t.Fatal("OnError left HasErrorHandler() false, so error_log would write to the process error stream")
	}

	fn, ok := rt.Callable("error_log")
	if !ok {
		t.Fatal("error_log is not registered")
	}

	got, err := fn("disk full")
	if err != nil {
		t.Fatalf("error_log: %v", err)
	}
	if got != true {
		t.Errorf("error_log() = %#v, want true", got)
	}

	if _, err := fn("second"); err != nil {
		t.Fatalf("error_log: %v", err)
	}

	want := []string{"disk full", "second"}
	if len(recorded) != len(want) {
		t.Fatalf("the handler received %v, want %v", recorded, want)
	}
	for i, message := range want {
		if recorded[i] != message {
			t.Errorf("message %d is %q, want %q", i, recorded[i], message)
		}
	}
}
