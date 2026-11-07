package files

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerStreams installs the file-handle functions. A handle is the *os.File
// fopen() returns; PHP calls it a resource.
func registerStreams(rt *runner.Runtime, r root) {
	// fopen opens $filename in $mode and returns a handle, or false on
	// failure; php://output is the script's own output stream, php://input the
	// raw request body (stdin under the cli SAPI), php://memory and php://temp
	// a buffer the script owns, and fopen refuses a mode that can write
	// outside writable_paths.
	rt.RegisterFunc("fopen", func(filename, mode string) (any, error) {
		if scheme, ok := strings.CutPrefix(filename, "php://"); ok {
			switch scheme {
			case "output":
				return outputStream{rt: rt}, nil
			case "input":
				return &inputStream{Reader: inputSource(rt)}, nil
			}
			if handle, ok, err := openWrapper(rt, scheme, mode); ok {
				if err != nil {
					return false, err
				}
				return handle, nil
			}
			// A scheme no wrapper answers for is false, PHP's answer for
			// a stream it cannot open: resolving the name would invent a
			// file called php:/filter instead. memory and temp are
			// wrappers of their own; see memory.go.
			return false, nil
		}
		name, isUpload := r.uploadPath(filename)
		if !isUpload {
			name = r.resolve(filename)
		}
		// fopen checks writable_paths only for a mode that can write. Opening
		// a file for reading is a read wherever it lives.
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
		case outputStream, *inputStream:
			// Closing the handle does not close the script's output or
			// its request body; PHP answers true for both.
			return true
		case io.Closer:
			// A wrapper's own handle, php://memory among them.
			return h.Close() == nil
		}
		return false
	})
	// stream_get_contents reads $stream whole, or $maxlength bytes from
	// $offset; a negative or absent one of either reads the rest from where the
	// handle sits. It takes any reader, as fwrite takes any writer: STDIN is
	// one.
	rt.RegisterFunc("stream_get_contents", func(stream io.Reader, bounds ...any) (string, error) {
		length, offset := streamBounds(bounds)
		if offset >= 0 {
			seeker, ok := stream.(io.Seeker)
			if !ok {
				return "", fmt.Errorf("stream_get_contents(): the stream does not seek")
			}
			if _, err := seeker.Seek(offset, io.SeekStart); err != nil {
				return "", err
			}
		}
		source := stream
		if length >= 0 {
			source = io.LimitReader(stream, length)
		}
		v, err := io.ReadAll(source)
		if err != nil {
			return "", err
		}
		return string(v), nil
	})
}

// outputStream is the handle fopen("php://output") returns. It holds the
// runtime and not the writer of the moment and every write lands wherever
// script output goes at the time: a handle opened before ob_start() writes into
// the buffer while one is active, exactly as echo would.
type outputStream struct {
	rt *runner.Runtime
}

func (o outputStream) Write(p []byte) (int, error) {
	return o.rt.Output().Write(p)
}

// inputStream is the handle fopen("php://input") returns. The named type is
// what lets fclose recognise it.
type inputStream struct {
	io.Reader
}

// inputSource returns what php://input reads, matching PHP's two SAPIs: a
// request answers with the buffered body, rewindable on every open as PHP
// 5.6+ made it, while the CLI maps it onto stdin, a live stream a script
// drains once. A staged body wins over the SAPI because the test harness
// speaks as cli, the way the php column does, while still carrying the
// request a fixture states; a cli run without one keeps its stdin.
func inputSource(rt *runner.Runtime) io.Reader {
	if c, ok := runner.RequestContext(rt.Context()); ok && len(c.RawBody()) > 0 {
		return bytes.NewReader(c.RawBody())
	}
	if rt.SAPI() == "cli" {
		if in := rt.Stdin(); in != nil {
			return in
		}
	}
	return bytes.NewReader(nil)
}

// openMode maps a PHP fopen() mode string onto the flags os.OpenFile takes. It
// takes the "b" suffix and ignores it, as PHP does on every platform it runs
// on but Windows.
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

// writes reports whether an fopen() mode can change the file. Only "r" cannot;
// every other mode either truncates, appends or allows a write on the handle.
func writes(mode string) bool {
	return strings.TrimSuffix(mode, "b") != "r"
}
