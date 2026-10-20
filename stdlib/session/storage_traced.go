package session

import (
	"context"
	"errors"
	"io/fs"
	"time"

	"github.com/titpetric/phpscript/telemetry"
)

// tracedStorage records a span per session store operation. It decorates
// whichever backend a script constructed, so a single wrapper instruments
// memory and disk alike, and the trace shows which one a request paid for.
//
// The span never carries the session ID. That ID is the credential in the
// cookie, and the debug front end is a place it must not turn up.
type tracedStorage struct {
	storage Storage
	backend string
}

var _ Storage = (*tracedStorage)(nil)

// traceStorage wraps storage for recording. It returns an already traced
// storage unchanged; a manager built from another manager's storage passes
// one in.
func traceStorage(storage Storage) Storage {
	if _, ok := storage.(*tracedStorage); ok {
		return storage
	}
	return &tracedStorage{storage: storage, backend: backendName(storage)}
}

// backendName is what the trace calls the storage behind the wrapper.
//
// Every span carries it, because the two backends promise different things of
// a session: a disk session outlives the worker and every worker reads it,
// where a memory session does neither. A manager that fell back to memory then
// says so on each operation, and not in the log line alone (session.go,
// defaultStorage).
func backendName(storage Storage) string {
	switch storage.(type) {
	case *StorageDisk:
		return "disk"
	case *StorageMemory:
		return "memory"
	default:
		// A storage a host implemented. Its type name would put a Go
		// identifier on a front end, so the trace says this much and no
		// more: neither of the two this package ships.
		return "custom"
	}
}

// Load reads session data, recording whether the session exists.
func (s *tracedStorage) Load(ctx context.Context, id string) ([]byte, error) {
	span := telemetry.StartSpan(ctx, "session load", telemetry.KindCache)
	defer span.End()
	span.SetAttribute("backend", s.backend)

	data, err := s.storage.Load(ctx, id)
	span.SetAttribute("hit", err == nil)
	span.SetAttribute("bytes", len(data))
	if telemetry.Recordable(err) && !missing(err) {
		span.RecordError(err)
	}
	return data, err
}

// Save writes session data.
func (s *tracedStorage) Save(ctx context.Context, id string, data []byte) error {
	span := telemetry.StartSpan(ctx, "session save", telemetry.KindCache)
	defer span.End()
	span.SetAttribute("backend", s.backend)
	span.SetAttribute("bytes", len(data))

	err := s.storage.Save(ctx, id, data)
	span.RecordError(err)
	return err
}

// Delete drops a session.
func (s *tracedStorage) Delete(ctx context.Context, id string) error {
	span := telemetry.StartSpan(ctx, "session delete", telemetry.KindCache)
	defer span.End()
	span.SetAttribute("backend", s.backend)

	err := s.storage.Delete(ctx, id)
	if telemetry.Recordable(err) && !missing(err) {
		span.RecordError(err)
	}
	return err
}

// missing reports whether an error means the session does not exist. An expired
// or never-created session is what a logged out visitor looks like, so the span
// records a miss and the trace no failure.
func missing(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// Prune drops sessions older than maxAge.
func (s *tracedStorage) Prune(ctx context.Context, maxAge time.Duration) error {
	span := telemetry.StartSpan(ctx, "session prune", telemetry.KindCache)
	defer span.End()
	span.SetAttribute("backend", s.backend)
	span.SetAttribute("max_age", maxAge.String())

	err := s.storage.Prune(ctx, maxAge)
	span.RecordError(err)
	return err
}
