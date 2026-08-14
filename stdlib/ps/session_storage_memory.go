package ps

import (
	"context"
	"io/fs"
	"sync"
	"time"
)

type memorySession struct {
	data    []byte
	savedAt time.Time
}

// SessionStorageMemory stores sessions in memory. Its contents are not durable.
type SessionStorageMemory struct {
	mu       sync.RWMutex
	sessions map[string]memorySession
}

// NewSessionStorageMemory creates empty in-memory session storage.
func NewSessionStorageMemory() *SessionStorageMemory {
	return &SessionStorageMemory{sessions: make(map[string]memorySession)}
}

// Load retrieves a copy of a session's data.
func (s *SessionStorageMemory) Load(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	session, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), session.data...), nil
}

// Save stores a copy of the session data.
func (s *SessionStorageMemory) Save(ctx context.Context, id string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = make(map[string]memorySession)
	}
	s.sessions[id] = memorySession{
		data:    append([]byte(nil), data...),
		savedAt: time.Now(),
	}
	s.mu.Unlock()
	return nil
}

// Delete removes a session from memory.
func (s *SessionStorageMemory) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return fs.ErrNotExist
	}
	delete(s.sessions, id)
	return nil
}

// Prune removes sessions that have not been saved within maxAge.
func (s *SessionStorageMemory) Prune(ctx context.Context, maxAge time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if session.savedAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
	return nil
}

var _ SessionStorage = (*SessionStorageMemory)(nil)
