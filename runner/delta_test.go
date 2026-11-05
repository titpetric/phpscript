package runner

import (
	"testing"
)

// TestDelta covers the empty report: an install that registers nothing
// leaves every list empty.
func TestDelta(t *testing.T) {
	t.Parallel()
	d := New(nil, Options{}).Track(func(*Runtime) {})
	if len(d.Funcs)+len(d.Replaced)+len(d.Classes)+len(d.Constants) != 0 {
		t.Fatalf("got %+v", d)
	}
}
