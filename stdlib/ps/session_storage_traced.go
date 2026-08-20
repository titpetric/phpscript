package ps

import (
	"context"
	"errors"
	"io/fs"
	"time"

	"github.com/titpetric/phpscript/telemetry"
)

// tracedSessionStorage records a span per session store operation. It decorates
// whichever backend a script constructed, so memory and disk are instrumented
// once here rather than in each implementation, and the trace shows which one a
// request paid for.
//
// The session ID is never recorded. It is the credential in the cookie, and the
// debug front end is a place it must not turn up.
type tracedSessionStorage struct {
	storage SessionStorage
}

var _ SessionStorage = (*tracedSessionStorage)(nil)

// traceSessionStorage wraps storage for recording. A storage that is already
// traced, which is what a manager built from another manager's storage would
// hand over, is returned as it is.
func traceSessionStorage(storage SessionStorage) SessionStorage {
	if _, ok := storage.(*tracedSessionStorage); ok {
		return storage
	}
	return &tracedSessionStorage{storage: storage}
}

// Load reads session data, recording whether the session was there.
func (s *tracedSessionStorage) Load(ctx context.Context, id string) ([]byte, error) {
	span := telemetry.StartSpan(ctx, "session load", telemetry.KindCache)
	defer span.End()

	data, err := s.storage.Load(ctx, id)
	span.SetAttribute("hit", err == nil)
	span.SetAttribute("bytes", len(data))
	if telemetry.Recordable(err) && !missing(err) {
		span.RecordError(err)
	}
	return data, err
}

// Save writes session data.
func (s *tracedSessionStorage) Save(ctx context.Context, id string, data []byte) error {
	span := telemetry.StartSpan(ctx, "session save", telemetry.KindCache)
	defer span.End()
	span.SetAttribute("bytes", len(data))

	err := s.storage.Save(ctx, id, data)
	span.RecordError(err)
	return err
}

// Delete drops a session.
func (s *tracedSessionStorage) Delete(ctx context.Context, id string) error {
	span := telemetry.StartSpan(ctx, "session delete", telemetry.KindCache)
	defer span.End()

	err := s.storage.Delete(ctx, id)
	if telemetry.Recordable(err) && !missing(err) {
		span.RecordError(err)
	}
	return err
}

// missing reports whether an error means the session was not there. An expired
// or never-created session is what a logged out visitor looks like, so it is a
// miss on the span rather than a failure on the trace.
func missing(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// Prune drops sessions older than maxAge.
func (s *tracedSessionStorage) Prune(ctx context.Context, maxAge time.Duration) error {
	span := telemetry.StartSpan(ctx, "session prune", telemetry.KindCache)
	defer span.End()
	span.SetAttribute("max_age", maxAge.String())

	err := s.storage.Prune(ctx, maxAge)
	span.RecordError(err)
	return err
}
