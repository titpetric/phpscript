package files

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerStreams installs the file-handle functions over *os.File handles.
func registerStreams(rt *runner.Runtime, r root) {
	// fopen opens $filename in $mode and returns a handle, or false on
	// failure. php://output is the script's output stream. php://input is the
	// raw request body, stdin in the cli SAPI. php://memory and php://temp are
	// buffers the script owns. fopen refuses a mode that can write outside
	// writable_paths.
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
			// separate wrappers; see memory.go.
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
		f, err := openFile(name, mode)
		if err != nil {
			return false, nil
		}
		return f, nil
	})
	// fwrite takes any writer, not just a handle fopen() produced: STDOUT and
	// STDERR are the process streams. A binding may pass PHP another io.Writer.
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
	// $offset. A negative or absent value reads the rest from the current
	// position. It takes any reader, as fwrite takes any writer; STDIN
	// qualifies.
	rt.RegisterFunc("stream_get_contents", func(stream io.Reader, bounds ...any) (string, error) {
		length, offset := streamBounds(bounds)
		if seeker, ok := stream.(io.Seeker); ok && offset >= 0 {
			// php reads from where the handle sits when the seek fails: a pipe
			// takes no offset.
			_, _ = seeker.Seek(offset, io.SeekStart)
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

// outputStream writes to the runtime's current Output, as echo would at call time.
type outputStream struct {
	rt *runner.Runtime
}

func (o outputStream) Write(p []byte) (int, error) {
	return o.rt.Output().Write(p)
}

// inputStream is the php://input handle, named so fclose recognises it.
type inputStream struct {
	io.Reader
}

// inputSource answers php://input from a staged body, else stdin.
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

// openMode maps a PHP fopen mode onto os.OpenFile flags.
func openMode(name, mode string) (*os.File, error) {
	switch strings.TrimSuffix(mode, "b") {
	case "r":
		// Read only, as PHP documents. A handle opened with "r" cannot write.
		// That mode lets writable_paths leave the path alone.
		return os.OpenFile(name, os.O_RDONLY, 0)
	case "r+":
		return os.OpenFile(name, os.O_RDWR, 0)
	case "a", "a+":
		return os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	default: // "w", "w+"
		return os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	}
}

// writes reports whether an fopen mode can change the file; only "r" cannot.
func writes(mode string) bool {
	return strings.TrimSuffix(mode, "b") != "r"
}
