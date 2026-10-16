package runner

import (
	"context"
)

// RequestState is what BeginRequest saves, held on the caller's frame and
// handed back to EndRequest. A value and no closure, because the restore
// closure EnterRequest builds is one heap allocation per request on the
// serving path.
type RequestState struct {
	client      context.Context
	clientDone  <-chan struct{}
	ignoreAbort bool
	inRequest   bool
}
