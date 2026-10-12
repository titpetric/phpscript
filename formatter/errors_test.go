package formatter

import (
	"errors"
	"fmt"
	"testing"
)

// TestSkipError covers the error a refused file carries. Formatting rewrites a
// file in place, so the message names the file as well as what was wrong with
// it, and Unwrap answers the reason, so a caller matches on the parse error
// itself and never on the wrapper.
func TestSkipError(t *testing.T) {
	reason := errors.New("unexpected token")
	err := &SkipError{Path: "src/index.php", Reason: reason}

	if got, want := err.Error(), "src/index.php: unexpected token"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, reason) {
		t.Error("errors.Is does not reach the reason, so Unwrap is not wired")
	}
	if got := err.Unwrap(); got != reason {
		t.Errorf("Unwrap() = %v, want the reason", got)
	}

	// Through a wrapper: a caller several frames up reads it this way.
	wrapped := fmt.Errorf("formatting: %w", err)
	var skip *SkipError
	if !errors.As(wrapped, &skip) {
		t.Fatal("errors.As did not find the SkipError through a wrapper")
	}
	if skip.Path != "src/index.php" {
		t.Errorf("the unwrapped error names %q", skip.Path)
	}
}
