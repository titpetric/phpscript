package core

import (
	"context"
	"fmt"
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
	rt.RegisterInfo("SharedMemory", sharedMemoryInfo)
}

// sharedMemoryInfo reports the store bound into the runtime, for phpinfo().
//
// A store outlives the requests that write to it, so none of it is in what
// memory_get_usage() answers. A host that bound none has nothing to report and
// the section does not print.
func sharedMemoryInfo(rt *runner.Runtime) []runner.InfoField {
	store, _ := rt.Context().Value(sharedMemoryKey{}).(*SharedMemory)
	if store == nil {
		return nil
	}
	entries, counters, bytes := store.Usage()
	return []runner.InfoField{
		{Name: "Entries", Value: strconv.Itoa(entries)},
		{Name: "Counters", Value: strconv.Itoa(counters)},
		{Name: "Size", Value: fmt.Sprintf("%.2f MiB", float64(bytes)/(1024*1024))},
	}
}

// Usage reports what the store holds: how many entries, how many counters, and
// the bytes of the keys and values behind them.
//
// The byte figure is the strings themselves, not the map overhead around them,
// which is the same basis memory_get_usage() reports a PHP value on.
func (s *SharedMemory) Usage() (entries, counters int, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range s.data {
		bytes += int64(len(key) + len(value))
	}
	for key := range s.counters {
		bytes += int64(len(key)) + 8
	}
	return len(s.data), len(s.counters), bytes
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
