package formatter

import (
	"errors"
	"testing"
)

// TestResult covers the two outcomes the formatter reports per file. Skipped
// carries the reason and is nil for a file that was formatted, so a caller
// reads one field to tell a rewrite from a refusal.
func TestResult(t *testing.T) {
	formatted := Result{Path: "a.php", Changed: true}
	if formatted.Skipped != nil {
		t.Errorf("a formatted file carries a skip reason: %v", formatted.Skipped)
	}

	reason := errors.New("parse error")
	skipped := Result{Path: "b.php", Skipped: reason}
	if skipped.Changed {
		t.Error("a skipped file is reported as changed")
	}
	if !errors.Is(skipped.Skipped, reason) {
		t.Errorf("Skipped = %v, want the reason it was given", skipped.Skipped)
	}

	// A file the formatter read and found nothing to change in is neither
	// changed nor skipped.
	unchanged := Result{Path: "c.php"}
	if unchanged.Changed || unchanged.Skipped != nil {
		t.Errorf("an unchanged file reports as %+v", unchanged)
	}
}
