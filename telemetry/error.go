package telemetry

import (
	"context"
	"database/sql"
	"errors"
)

// Recordable reports whether an error is worth failing a span over.
// sql.ErrNoRows and context.Canceled are control flow and not failures.
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
