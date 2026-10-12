package tests

import (
	"context"
)

// WithWorker marks ctx as belonging to worker id, so a fixture run under it
// takes that worker's caches. The command's worker loop calls this once per
// goroutine; everything else inherits worker zero.
func WithWorker(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, workerKey, id)
}
