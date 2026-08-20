package fs

import (
	"io"
	"os"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerStreams installs the file-handle functions. A handle is the *os.File
// fopen() returns; PHP calls it a resource.
func registerStreams(rt *runner.Runtime, r root) {
	rt.RegisterFunc("fopen", func(p, mode string) any {
		f, err := openMode(r.resolve(p), mode)
		if err != nil {
			return false
		}
		return f
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
	case "r", "r+":
		return os.OpenFile(name, os.O_RDWR, 0)
	case "a", "a+":
		return os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	default: // "w", "w+"
		return os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	}
}
