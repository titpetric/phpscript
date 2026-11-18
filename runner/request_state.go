package runner

import (
	"context"
)

// RequestState holds what BeginRequest saves, on the caller's frame and handed
// back to EndRequest. A plain value, since the restore closure EnterRequest
// builds would cost one heap allocation per request on the serving path.
type RequestState struct {
	client      context.Context
	clientDone  <-chan struct{}
	ignoreAbort bool
	inRequest   bool
}
