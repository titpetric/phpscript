package files

import (
	"io"
	"os"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerStreams installs the file-handle functions. A handle is the *os.File
// fopen() returns; PHP calls it a resource.
func registerStreams(rt *runner.Runtime, r root) {
	// fopen opens $filename in $mode and returns a handle, or false on failure; php://output is the script's own output stream, and a mode that can write is refused outside writable_paths.
	rt.RegisterFunc("fopen", func(filename, mode string) (any, error) {
		if scheme, ok := strings.CutPrefix(filename, "php://"); ok {
			if scheme == "output" {
				return outputStream{rt: rt}, nil
			}
			// The other wrappers (input, memory, temp) are not
			// implemented; false is PHP's answer for a stream it cannot
			// open, and resolving the name would invent a file called
			// php:/input instead.
			return false, nil
		}
		name := r.resolve(filename)
		// Only a mode that can write is held to writable_paths. Opening a
		// file for reading is a read wherever it lives.
		if writes(mode) {
			if _, err := r.resolveWrite("fopen", filename); err != nil {
				return false, err
			}
		}
		f, err := openMode(name, mode)
		if err != nil {
			return false, nil
		}
		return f, nil
	})
	// fwrite takes any writer, not just a handle fopen() produced: STDOUT and
	// STDERR are the process's own streams, and a binding is free to hand PHP
	// an io.Writer of its own.
	rt.RegisterFunc("fwrite", func(stream io.Writer, s string) any {
		if stream == nil {
			return false
		}
		n, err := io.WriteString(stream, s)
		if err != nil {
			return false
		}
		return int64(n)
	})
	// fclose closes a handle fopen() returned and reports whether the close succeeded.
	rt.RegisterFunc("fclose", func(f any) bool {
		switch h := f.(type) {
		case *os.File:
			if h == nil {
				return false
			}
			return h.Close() == nil
		case outputStream:
			// Closing the handle does not close the script's output;
			// PHP answers true and the next echo still prints.
			return true
		}
		return false
	})
	// stream_get_contents reads any reader for the same reason fwrite writes
	// any writer: STDIN is one, and it is the usual way a script reads a whole
	// handle at once.
	rt.RegisterFunc("stream_get_contents", func(stream io.Reader, _ ...any) (string, error) {
		v, err := io.ReadAll(stream)
		if err != nil {
			return "", err
		}
		return string(v), nil
	})
}

// outputStream is the handle fopen("php://output") returns. It holds the
// runtime rather than the writer of the moment, so every write lands wherever
// script output currently goes: a handle opened before ob_start() writes into
// the buffer while one is active, exactly as echo would.
type outputStream struct {
	rt *runner.Runtime
}

func (o outputStream) Write(p []byte) (int, error) {
	return o.rt.Output().Write(p)
}

// openMode maps a PHP fopen() mode string onto the flags os.OpenFile takes. The
// "b" suffix is accepted and ignored, as it is on every platform PHP runs on
// that is not Windows.
func openMode(name, mode string) (*os.File, error) {
	switch strings.TrimSuffix(mode, "b") {
	case "r":
		// Read only, as PHP documents it. A handle opened with "r" cannot
		// write, which is also what lets writable_paths leave it alone.
		return os.OpenFile(name, os.O_RDONLY, 0)
	case "r+":
		return os.OpenFile(name, os.O_RDWR, 0)
	case "a", "a+":
		return os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	default: // "w", "w+"
		return os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	}
}

// writes reports whether an fopen() mode can modify the file. Only "r" cannot;
// every other mode either truncates, appends or allows a write on the handle.
func writes(mode string) bool {
	return strings.TrimSuffix(mode, "b") != "r"
}
