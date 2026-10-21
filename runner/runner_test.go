package runner

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// TestRuntime_RecordError covers the failure a script has no throw to catch:
// one that happened around the script and no statement in it, a request body
// refused for its size being the case it covers. It records and
// returns, so the host sees it and PHP execution is not unwound.
func TestRuntime_RecordError(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	var seen []error
	rt.OnError(func(err error) { seen = append(seen, err) })
	if !rt.HasErrorHandler() {
		t.Fatal("HasErrorHandler is false after OnError installed one")
	}

	refused := errors.New("the request body is over the limit")
	rt.RecordError(refused)
	if len(seen) != 1 || !errors.Is(seen[0], refused) {
		t.Fatalf("the handler received %v, want the one error", seen)
	}

	// A nil error is nothing to report, so it never reaches the handler.
	rt.RecordError(nil)
	if len(seen) != 1 {
		t.Errorf("the handler received %d errors, want the nil to have been dropped", len(seen))
	}

	// A script sees nothing of it: the program runs to its end.
	var out strings.Builder
	rt.ResetSession(&out, nil)
	program, err := rt.Load(`<?php echo "ran";`)
	if err != nil {
		t.Fatal(err)
	}
	rt.RecordError(errors.New("recorded before the run"))
	if err := rt.Run(program); err != nil {
		t.Errorf("a recorded error ended the run: %v", err)
	}
	if got := out.String(); got != "ran" {
		t.Errorf("output = %q, want the program to have run", got)
	}
}
