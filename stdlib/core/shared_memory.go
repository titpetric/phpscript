package core

import (
	"context"
	"strconv"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

type sharedMemoryKey struct{}

// SharedMemory is a thread-safe in-memory key-value and atomic counter store
// exposed as SharedMemory to scripts.
type SharedMemory struct {
	mu       sync.Mutex
	data     map[string]string
	counters map[string]int64
}

// NewSharedMemory returns a new empty SharedMemory instance.
func NewSharedMemory() *SharedMemory {
	return &SharedMemory{
		data:     make(map[string]string),
		counters: make(map[string]int64),
	}
}

// SharedMemoryContext binds an SharedMemory instance into the context.
func SharedMemoryContext(ctx context.Context, s *SharedMemory) context.Context {
	return context.WithValue(ctx, sharedMemoryKey{}, s)
}

// NewSharedMemoryBinding is a key-value and counter store shared across
// requests: `new SharedMemory` returns the store the host bound into the
// runtime context, or a fresh empty store when none is bound.
func NewSharedMemoryBinding(ctx context.Context) (*SharedMemory, error) {
	s, _ := ctx.Value(sharedMemoryKey{}).(*SharedMemory)
	if s == nil {
		return NewSharedMemory(), nil
	}
	return s, nil
}

// init contributes the SharedMemory binding to stdlib.Register.
func init() {
	runner.RegisterBinding(RegisterSharedMemory)
}

// RegisterSharedMemory installs SharedMemory in the runtime.
func RegisterSharedMemory(rt *runner.Runtime) {
	rt.RegisterConstructor("SharedMemory", NewSharedMemoryBinding)
}

// Set stores a string value.
func (s *SharedMemory) Set(_ context.Context, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get retrieves a string value by key, or returns empty string if missing.
func (s *SharedMemory) Get(_ context.Context, key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key]
}

// Incr atomically increments and returns a counter.
func (s *SharedMemory) Incr(_ context.Context, key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[key]++
	return s.counters[key]
}

// Count returns a counter as a formatted string.
func (s *SharedMemory) Count(_ context.Context, key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strconv.FormatInt(s.counters[key], 10)
}

// Delete removes a key from storage.
func (s *SharedMemory) Delete(_ context.Context, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existsData := s.data[key]
	_, existsCount := s.counters[key]
	delete(s.data, key)
	delete(s.counters, key)
	return existsData || existsCount
}

// Has checks if a key exists in data or counters.
func (s *SharedMemory) Has(_ context.Context, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok1 := s.data[key]
	_, ok2 := s.counters[key]
	return ok1 || ok2
}

// Clear resets all keys and counters.
func (s *SharedMemory) Clear(_ context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]string)
	s.counters = make(map[string]int64)
}
