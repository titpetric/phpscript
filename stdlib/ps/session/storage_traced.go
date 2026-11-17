package session

import (
	"context"
	"errors"
	"io/fs"
	"time"

	"github.com/titpetric/phpscript/telemetry"
)

// tracedStorage records a span per session store operation without ever recording the session ID.
type tracedStorage struct {
	storage Storage
	backend string
}

var _ Storage = (*tracedStorage)(nil)

// traceStorage wraps storage for recording, returning an already traced storage unchanged.
func traceStorage(storage Storage) Storage {
	if _, ok := storage.(*tracedStorage); ok {
		return storage
	}
	return &tracedStorage{storage: storage, backend: backendName(storage)}
}

// backendName names the storage behind the wrapper as disk, memory or custom.
func backendName(storage Storage) string {
	switch storage.(type) {
	case *StorageDisk:
		return "disk"
	case *StorageMemory:
		return "memory"
	default:
		// A storage a host implemented. Its type name would put a Go
		// identifier on a front end and the trace says this much and no
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

// DeleteByUser drops every session one user holds, recording how many.
func (s *tracedStorage) DeleteByUser(ctx context.Context, userID string) (int, error) {
	span := telemetry.StartSpan(ctx, "session delete_by_user", telemetry.KindCache)
	defer span.End()
	span.SetAttribute("backend", s.backend)

	taken, err := s.storage.DeleteByUser(ctx, userID)
	span.SetAttribute("sessions", taken)
	span.RecordError(err)
	return taken, err
}

// missing reports whether an error means the session does not exist.
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
