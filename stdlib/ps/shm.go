package ps

import (
	"context"
	"strconv"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

type shmKey struct{}

// SHM is a thread-safe in-memory key-value and atomic counter store
// exposed as PS\SHM to scripts.
type SHM struct {
	mu       sync.Mutex
	data     map[string]string
	counters map[string]int64
}

// NewSHM returns a new empty SHM instance.
func NewSHM() *SHM {
	return &SHM{
		data:     make(map[string]string),
		counters: make(map[string]int64),
	}
}

// SHMContext binds an SHM instance into the context.
func SHMContext(ctx context.Context, s *SHM) context.Context {
	return context.WithValue(ctx, shmKey{}, s)
}

// NewSHMBinding is the constructor callback registered for PS\SHM.
func NewSHMBinding(ctx context.Context) (*SHM, error) {
	s, _ := ctx.Value(shmKey{}).(*SHM)
	if s == nil {
		return NewSHM(), nil
	}
	return s, nil
}

// RegisterSHM installs PS\SHM and SharedMemory in the runtime.
func RegisterSHM(rt *runner.Runtime) {
	rt.RegisterConstructor("PS\\SHM", NewSHMBinding)
	rt.RegisterConstructor("SharedMemory", NewSHMBinding)
}

// Set stores a string value.
func (s *SHM) Set(_ context.Context, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get retrieves a string value by key, or returns empty string if missing.
func (s *SHM) Get(_ context.Context, key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key]
}

// Incr atomically increments and returns a counter.
func (s *SHM) Incr(_ context.Context, key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[key]++
	return s.counters[key]
}

// Count returns a counter as a formatted string.
func (s *SHM) Count(_ context.Context, key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strconv.FormatInt(s.counters[key], 10)
}

// Delete removes a key from storage.
func (s *SHM) Delete(_ context.Context, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existsData := s.data[key]
	_, existsCount := s.counters[key]
	delete(s.data, key)
	delete(s.counters, key)
	return existsData || existsCount
}

// Has checks if a key exists in data or counters.
func (s *SHM) Has(_ context.Context, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok1 := s.data[key]
	_, ok2 := s.counters[key]
	return ok1 || ok2
}

// Clear resets all keys and counters.
func (s *SHM) Clear(_ context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]string)
	s.counters = make(map[string]int64)
}
