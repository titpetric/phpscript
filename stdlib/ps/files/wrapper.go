package files

import (
	"io"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// Wrapper opens the handle fopen("php://<scheme>") answers for a scheme added
// with RegisterWrapper, so a host adds stdout, stderr or memory without editing
// fopen. The handle is whatever the stream functions accept: an *os.File, or a
// value that is an io.Reader, io.Writer or io.Seeker as the call needs. A scheme
// is matched without regard to case, a later registration wins, and output and
// input stay the runtime's own.
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

// readStream reads a php:// scheme whole, as file_get_contents does. A fresh
// buffer answers the empty string; a scheme no wrapper opens answers false.
func readStream(rt *runner.Runtime, scheme string) any {
	handle, ok, err := openWrapper(rt, scheme, "r")
	if !ok || err != nil {
		return false
	}
	reader, ok := handle.(io.Reader)
	if !ok {
		return false
	}
	b, err := io.ReadAll(reader)
	if err != nil {
		return false
	}
	return string(b)
}

// writeStream writes data to a php:// scheme and answers the bytes written.
// php://output reaches script output; a wrapper's buffer takes the write and goes.
func writeStream(rt *runner.Runtime, scheme, data string) any {
	var handle any = outputStream{rt: rt}
	if !strings.EqualFold(scheme, "output") {
		opened, ok, err := openWrapper(rt, scheme, "w")
		if !ok || err != nil {
			return false
		}
		handle = opened
	}
	writer, ok := handle.(io.Writer)
	if !ok {
		return false
	}
	n, err := io.WriteString(writer, data)
	if err != nil {
		return false
	}
	return int64(n)
}

// openWrapper opens the handle for scheme, and reports whether one is bound.
// A scheme php spells with an argument, `temp/maxmemory:1024`, resolves on the
// name ahead of the first slash.
func openWrapper(rt *runner.Runtime, scheme, mode string) (handle any, ok bool, err error) {
	name := strings.ToLower(scheme)
	wrappers.RLock()
	open, found := wrappers.byScheme[name]
	if !found {
		if head, _, cut := strings.Cut(name, "/"); cut {
			open, found = wrappers.byScheme[head]
		}
	}
	wrappers.RUnlock()
	if !found {
		return nil, false, nil
	}
	handle, err = open(rt, mode)
	return handle, true, err
}
