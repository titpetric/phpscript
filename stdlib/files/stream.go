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
	rt.RegisterFunc("fopen", func(p, mode string) (any, error) {
		name := r.resolve(p)
		// Only a mode that can write is held to writable_paths. Opening a
		// file for reading is a read wherever it lives.
		if writes(mode) {
			if _, err := r.resolveWrite("fopen", p); err != nil {
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
	rt.RegisterFunc("fclose", func(f *os.File) bool {
		if f == nil {
			return false
		}
		return f.Close() == nil
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
