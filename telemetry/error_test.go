package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

// TestRecordable covers which errors fail a span. A query that found no rows
// and a request the client hung up on are control flow, and failing a span over
// one would fail the trace and the SLA computed from it. A deadline that
// elapsed is a failure: the work did not finish. The test is errors.Is and a
// sentinel wrapped by a caller reads the same as a bare one.
func TestRecordable(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nothing failed", err: nil},
		{name: "no rows", err: sql.ErrNoRows},
		{name: "no rows, wrapped", err: fmt.Errorf("get user: %w", sql.ErrNoRows)},
		{name: "the client left", err: context.Canceled},
		{name: "the client left, wrapped", err: fmt.Errorf("serving: %w", context.Canceled)},
		{name: "the deadline elapsed", err: context.DeadlineExceeded, want: true},
		{name: "a syntax error", err: errors.New(`near "slect": syntax error`), want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := Recordable(test.err); got != test.want {
				t.Errorf("Recordable(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}
