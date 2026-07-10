package tests

import (
	"context"
	"errors"
	"sort"
)

// ---------------------------------------------------------------------------
// Host-provided capability surfaced to PHP
// ---------------------------------------------------------------------------

// Storage is a host-provided key/value capability whose context parameters are
// supplied automatically when PHP invokes its methods.
type Storage interface {
	Set(ctx context.Context, key, value string)
	Get(ctx context.Context, key string) (Record, error)
	All(ctx context.Context) ([]Record, error)
	Len() int64
	Tenant() string
}

// Record is a rich value type returned to PHP. Its exported fields are read from
// PHP via property access (`$rec->key`, `$rec->value`), matched case-insensitively.
type Record struct {
	Key   string
	Value string
}

// memStorage is an in-memory Storage implementation.
type memStorage struct {
	data   map[string]string
	tenant string
}

func (s *memStorage) Set(_ context.Context, key, value string) { s.data[key] = value }

// Get returns (Record, error): a rich struct value (not a scalar). The error is
// omitted on the PHP side (handled as a throw); the Record is assigned to the
// PHP variable, whose fields are then read with `->`.
func (s *memStorage) Get(_ context.Context, key string) (Record, error) {
	v, ok := s.data[key]
	if !ok {
		return Record{}, errors.New("storage: missing key " + key)
	}
	return Record{Key: key, Value: v}, nil
}

// All returns a list of rich types ([]Record), key-sorted for determinism, so
// PHP can foreach over a Go slice and read struct fields on each element.
func (s *memStorage) All(_ context.Context) ([]Record, error) {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Record, 0, len(keys))
	for _, k := range keys {
		out = append(out, Record{Key: k, Value: s.data[k]})
	}
	return out, nil
}

func (s *memStorage) Len() int64     { return int64(len(s.data)) }
func (s *memStorage) Tenant() string { return s.tenant }

// ctxKey is the context key used to thread request-scoped data into constructors.
type ctxKey string

const tenantKey ctxKey = "tenant"

// NewStorage is the constructor registered for `new Storage`. Its first
// parameter is a context.Context, filled in automatically by the runner, so PHP
// calls `new Storage` with no arguments.
func NewStorage(ctx context.Context) (Storage, error) {
	if ctx == nil {
		return nil, errors.New("storage: nil context")
	}
	tenant, _ := ctx.Value(tenantKey).(string)
	return &memStorage{data: map[string]string{}, tenant: tenant}, nil
}

// NewFailStorage is a constructor that always fails, used to exercise the
// thrown-error path of `new`.
func NewFailStorage(ctx context.Context) (Storage, error) {
	return nil, errors.New("boom")
}
