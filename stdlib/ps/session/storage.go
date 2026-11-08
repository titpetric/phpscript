package session

import (
	"context"
	"time"
)

// Storage persists opaque session data by ID.
//
// DeleteByUser drops every session a user holds and answers how many it took.
// A session holds its user's primary key as its whole payload (Manager.Start),
// and a store matches on that and needs no index.
type Storage interface {
	Load(ctx context.Context, id string) ([]byte, error)
	Save(ctx context.Context, id string, data []byte) error
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID string) (int, error)
	Prune(ctx context.Context, maxAge time.Duration) error
}
