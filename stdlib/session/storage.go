package session

import (
	"context"
	"time"
)

// Storage persists opaque session data by ID.
type Storage interface {
	Load(ctx context.Context, id string) ([]byte, error)
	Save(ctx context.Context, id string, data []byte) error
	Delete(ctx context.Context, id string) error
	Prune(ctx context.Context, maxAge time.Duration) error
}
