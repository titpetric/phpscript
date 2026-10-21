package tests

import (
	"context"
)

// WithWorker marks ctx as belonging to worker id, whose caches it takes.
func WithWorker(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, workerKey, id)
}
