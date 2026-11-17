package runner

import (
	"io"
	"testing"
)

// TestRuntime_GoRuntime covers the runtime mode the accessor reports.
func TestRuntime_GoRuntime(t *testing.T) {
	if New(io.Discard, Options{}).GoRuntime() {
		t.Error("the default runtime is go")
	}
	if !New(io.Discard, Options{Runtime: RuntimeGo}).GoRuntime() {
		t.Error("the go runtime is not go")
	}
}
