package runner

import (
	"bytes"
	"io"
	"testing"
)

// TestRuntime_PushOutput covers nesting a redirection. The innermost writer
// takes the text, so a captured region can itself capture. PHP's ob_* family in
// stdlib/compat is built on that nesting.
func TestRuntime_PushOutput(t *testing.T) {
	t.Parallel()
	var outer, inner bytes.Buffer
	base := &bytes.Buffer{}
	rt := New(base, Options{})

	rt.PushOutput(&outer)
	if _, err := io.WriteString(rt.Output(), "outer"); err != nil {
		t.Fatalf("writing to the captured output: %v", err)
	}

	rt.PushOutput(&inner)
	if _, err := io.WriteString(rt.Output(), "inner"); err != nil {
		t.Fatalf("writing to the nested output: %v", err)
	}

	if got, want := inner.String(), "inner"; got != want {
		t.Errorf("the innermost writer holds %q, want %q", got, want)
	}
	if got, want := outer.String(), "outer"; got != want {
		t.Errorf("the enclosing writer holds %q, want %q", got, want)
	}
	if base.Len() != 0 {
		t.Errorf("the runtime writer holds %q while a redirection is active", base.String())
	}
}

// TestRuntime_PopOutput covers ending the innermost redirection. A pop with
// nothing pushed reports false, so a script calling ob_end_clean() without a
// buffer open is told so; a nil push still leaves something to pop, because
// dropping the level would unstack the caller's writer on the next pop.
func TestRuntime_PopOutput(t *testing.T) {
	t.Parallel()
	var captured bytes.Buffer
	base := &bytes.Buffer{}
	rt := New(base, Options{})

	if rt.PopOutput() {
		t.Error("PopOutput() = true with no redirection active")
	}

	rt.PushOutput(&captured)
	rt.PushOutput(nil)
	if rt.Output() == io.Writer(&captured) {
		t.Error("a nil push left the enclosing writer innermost")
	}

	if !rt.PopOutput() {
		t.Error("PopOutput() = false after a nil push")
	}
	if rt.Output() != io.Writer(&captured) {
		t.Error("the enclosing writer is not innermost after the pop")
	}
	if !rt.PopOutput() {
		t.Error("PopOutput() = false with one redirection left")
	}
	if rt.Output() != io.Writer(base) {
		t.Error("Output() is not the runtime writer after the last pop")
	}
}

// TestRuntime_OutputDepth covers the count stdlib/compat reads to answer
// ob_get_level(). releaseOutput drops every level for a runtime answering one
// request after another, so the next request opens at zero.
func TestRuntime_OutputDepth(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	if got := rt.OutputDepth(); got != 0 {
		t.Errorf("OutputDepth() = %d on a fresh runtime, want 0", got)
	}

	rt.PushOutput(&bytes.Buffer{})
	rt.PushOutput(&bytes.Buffer{})
	if got := rt.OutputDepth(); got != 2 {
		t.Errorf("OutputDepth() = %d after two pushes, want 2", got)
	}

	rt.PopOutput()
	if got := rt.OutputDepth(); got != 1 {
		t.Errorf("OutputDepth() = %d after a pop, want 1", got)
	}

	rt.releaseOutput()
	if got := rt.OutputDepth(); got != 0 {
		t.Errorf("OutputDepth() = %d after releaseOutput, want 0", got)
	}
}

// TestRuntime_Output covers the writer every echo, every piece of inline HTML
// and every builtin emitting text writes to. A runtime with no redirection
// answers the writer behind it, so the text joins the response body in
// the order the script writes it.
func TestRuntime_Output(t *testing.T) {
	t.Parallel()
	base := &bytes.Buffer{}
	rt := New(base, Options{})
	if rt.Output() != io.Writer(base) {
		t.Error("Output() is not the writer the runtime was built on")
	}

	if _, err := io.WriteString(rt.Output(), "body"); err != nil {
		t.Fatalf("writing to the runtime output: %v", err)
	}
	if got, want := base.String(), "body"; got != want {
		t.Errorf("the runtime writer holds %q, want %q", got, want)
	}
}
