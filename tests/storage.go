package tests

import (
	"context"
	"errors"
	"sort"
)

// ---------------------------------------------------------------------------
// Host-provided capability surfaced to PHP
// ---------------------------------------------------------------------------

// Storage is a host-provided key/value capability; the runner fills its
// context parameters when PHP invokes its methods.
type Storage interface {
	Set(ctx context.Context, key, value string)
	Get(ctx context.Context, key string) (Record, error)
	All(ctx context.Context) ([]Record, error)
	Len() int64
	Tenant() string
}

// Record is a rich value type returned to PHP. PHP reads its exported fields
// through property access (`$rec->key`, `$rec->value`), case-insensitively.
type Record struct {
	Key   string
	Value string
}

// memStorage is an in-memory Storage for one runtime at a time, unlocked.
type memStorage struct {
	data   map[string]string
	tenant string
}

func (s *memStorage) Set(_ context.Context, key, value string) { s.data[key] = value }

// Get returns the Record for key, errors arriving as PHP throws.
func (s *memStorage) Get(_ context.Context, key string) (Record, error) {
	v, ok := s.data[key]
	if !ok {
		return Record{}, errors.New("storage: missing key " + key)
	}
	return Record{Key: key, Value: v}, nil
}

// All returns every Record key-sorted, for determinism on the PHP side.
func (s *memStorage) All(_ context.Context) ([]Record, error) {
	keys := s.sortedKeys()
	out := make([]Record, 0, len(keys))
	for _, k := range keys {
		out = append(out, Record{Key: k, Value: s.data[k]})
	}
	return out, nil
}

func (s *memStorage) Len() int64 { return int64(len(s.data)) }

func (s *memStorage) Tenant() string { return s.tenant }

// Keep returns the records the predicate accepts, taking the callback in Go terms.
func (s *memStorage) Keep(keep func(key, value string) bool) []Record {
	out := make([]Record, 0, len(s.data))
	for _, key := range s.sortedKeys() {
		if keep(key, s.data[key]) {
			out = append(out, Record{Key: key, Value: s.data[key]})
		}
	}
	return out
}

// Walk visits each record using the uniform callable shape, whose error ends it.
func (s *memStorage) Walk(visit func(...any) (any, error)) error {
	for _, key := range s.sortedKeys() {
		if _, err := visit(key, s.data[key]); err != nil {
			return err
		}
	}
	return nil
}

func (s *memStorage) sortedKeys() []string {
	keys := make([]string, 0, len(s.data))
	for key := range s.data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ctxKey is the context key that threads request-scoped data into constructors.
type ctxKey string

const tenantKey ctxKey = "tenant"

// NewStorage is the constructor registered for `new Storage`. Its first
// parameter is a context.Context, filled in automatically by the runner.
// PHP calls `new Storage` without arguments.
func NewStorage(ctx context.Context) (Storage, error) {
	if ctx == nil {
		return nil, errors.New("storage: nil context")
	}
	tenant, _ := ctx.Value(tenantKey).(string)
	return &memStorage{data: map[string]string{}, tenant: tenant}, nil
}

// NewFailStorage always fails, for the thrown-error path of `new`.
func NewFailStorage(ctx context.Context) (Storage, error) {
	return nil, errors.New("boom")
}
