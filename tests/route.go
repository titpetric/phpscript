package tests

import (
	"context"
	"errors"
	"strconv"
	"sync"
)

// SharedMemory is a host-owned in-memory key/value and counter store exposed to
// route PHP scripts. It behaves like request-shared SHM: each request gets a new
// VM, but all VMs can use the same Go value through `new SharedMemory`.
type SharedMemory struct {
	mu       sync.Mutex
	data     map[string]string
	counters map[string]int64
}

type sharedMemoryKey struct{}

// NewSharedMemory returns an empty shared-memory store.
func NewSharedMemory() *SharedMemory {
	return &SharedMemory{data: map[string]string{}, counters: map[string]int64{}}
}

// SharedMemoryContext stores shm on ctx for constructor injection.
func SharedMemoryContext(ctx context.Context, shm *SharedMemory) context.Context {
	return context.WithValue(ctx, sharedMemoryKey{}, shm)
}

// NewSharedMemoryBinding returns the constructor registered for PHP
// `new SharedMemory`.
func NewSharedMemoryBinding(ctx context.Context) (*SharedMemory, error) {
	shm, _ := ctx.Value(sharedMemoryKey{}).(*SharedMemory)
	if shm == nil {
		return nil, errors.New("shared memory: missing context value")
	}
	return shm, nil
}

// Set writes a string value.
func (m *SharedMemory) Set(_ context.Context, key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

// Get reads a string value. Missing keys return an empty string.
func (m *SharedMemory) Get(_ context.Context, key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[key]
}

// Incr increments and returns a named counter.
func (m *SharedMemory) Incr(_ context.Context, key string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[key]++
	return m.counters[key]
}

// Count returns a named counter as a string for simple PHP output checks.
func (m *SharedMemory) Count(_ context.Context, key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strconv.FormatInt(m.counters[key], 10)
}
