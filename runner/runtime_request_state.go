package runner

import (
	"context"
)

// BeginRequest is EnterRequest with the saved state as its answer; EndRequest
// puts it back.
func (rt *Runtime) BeginRequest(ctx context.Context) RequestState {
	saved := RequestState{
		client:      rt.client,
		clientDone:  rt.clientDone,
		ignoreAbort: rt.ignoreAbort,
		inRequest:   rt.inRequest,
	}

	rt.client = ctx
	rt.clientDone = watchEnd(ctx)
	// Off for every request and a handler that ignored a disconnect does not
	// decide it for the next one. The limit is not saved: the limit is the
	// runtime's and a handler moving it means to move it.
	rt.ignoreAbort = false
	rt.inRequest = true
	rt.refreshDeadlineArmed()
	return saved
}

// EndRequest restores what BeginRequest saved.
func (rt *Runtime) EndRequest(saved RequestState) {
	rt.client, rt.clientDone, rt.ignoreAbort, rt.inRequest = saved.client, saved.clientDone, saved.ignoreAbort, saved.inRequest
	rt.refreshDeadlineArmed()
}
