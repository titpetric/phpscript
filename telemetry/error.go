package telemetry

import (
	"context"
	"database/sql"
	"errors"
)

// Recordable reports whether an error is worth failing a span over. A query
// that found no rows and a request the client hung up on are control flow, not
// failures, and marking them would fail the trace and the recorded SLA with it.
//
// It is the shared answer to a question every traced binding asks, so that a
// cache miss and a canceled request read the same way on the front end whether
// they came from a query or from session storage.
func Recordable(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, sql.ErrNoRows):
		return false
	case errors.Is(err, context.Canceled):
		return false
	default:
		return true
	}
}
