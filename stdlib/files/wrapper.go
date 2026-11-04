package files

import (
	"strings"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// Wrapper opens the handle fopen("php://<scheme>") answers for a scheme added
// with RegisterWrapper, so a host adds stdout, stderr or memory without editing
// fopen. The handle is whatever the stream functions accept: an *os.File, or a
// value that is an io.Reader, io.Writer or io.Seeker as the call needs. A scheme
// is matched without regard to case, a second registration replaces the first,
// and output and input stay the runtime's own.
type Wrapper func(rt *runner.Runtime, mode string) (any, error)

var wrappers = struct {
	sync.RWMutex
	byScheme map[string]Wrapper
}{byScheme: map[string]Wrapper{}}

// RegisterWrapper binds open to the php:// stream named scheme.
func RegisterWrapper(scheme string, open Wrapper) {
	wrappers.Lock()
	defer wrappers.Unlock()
	wrappers.byScheme[strings.ToLower(scheme)] = open
}

// openWrapper opens the handle for scheme, and reports whether one is bound.
func openWrapper(rt *runner.Runtime, scheme, mode string) (handle any, ok bool, err error) {
	wrappers.RLock()
	open, found := wrappers.byScheme[strings.ToLower(scheme)]
	wrappers.RUnlock()
	if !found {
		return nil, false, nil
	}
	handle, err = open(rt, mode)
	return handle, true, err
}
