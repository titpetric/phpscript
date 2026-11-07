package files

import (
	"errors"
	"io"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/runner"
)

// php://memory and php://temp are the handles a script opens for a buffer it
// owns, with no file behind either: a CSV written before the response, a zip
// assembled in pieces, a body read twice. php's temp spills to a temporary
// file past a threshold, which no call through the handle API can observe, and
// the `php://temp/maxmemory:1024` suffix it takes resolves to the same buffer.
func init() {
	RegisterWrapper("memory", openMemory)
	RegisterWrapper("temp", openMemory)
}

// errStreamClosed reports a call on a closed handle. php throws a TypeError
// naming the argument; fwrite, fread and fseek each answer their own failure.
var errStreamClosed = errors.New("fopen: the stream is closed")

// errStreamReadOnly reports the write php refuses on a handle
// `fopen("php://memory", "r")` opened.
var errStreamReadOnly = errors.New("fopen: the stream is open for reading")

// streamBounds reads both optional arguments of stream_get_contents: the most
// bytes to read and where to read them from. An absent or a null one of either
// answers -1, which reads the rest from where the handle sits.
func streamBounds(args []any) (length, offset int64) {
	read := func(i int) int64 {
		if i >= len(args) || args[i] == nil {
			return -1
		}
		return phpval.Int(args[i])
	}
	return read(0), read(1)
}

// openMemory answers the handle for both schemes. php reads the
// `temp/maxmemory:1024` argument and refuses a name carrying anything else.
func openMemory(_ *runner.Runtime, scheme, mode string) (any, error) {
	if name, arg, cut := strings.Cut(strings.ToLower(scheme), "/"); cut {
		if name != "temp" || !strings.HasPrefix(arg, "maxmemory:") {
			return nil, nil
		}
	}
	writable, appends := memoryMode(mode)
	return &memoryStream{writable: writable, appends: appends}, nil
}

// memoryMode reads an fopen mode: whether a fresh buffer takes a write, and
// whether every write lands at the end. php's own wrapper refuses the write
// for `r`, `x` and `c` without a `+`, where `w` and `a` take one.
func memoryMode(mode string) (writable, appends bool) {
	mode = strings.ReplaceAll(strings.TrimSpace(mode), "b", "")
	appends = strings.HasPrefix(mode, "a")
	return appends || strings.HasPrefix(mode, "w") || strings.Contains(mode, "+"), appends
}

// memoryStream is a byte buffer behind a handle: one slice, a position the
// seek functions move, a write that pads the gap a seek past the end left.
type memoryStream struct {
	buf      []byte
	pos      int64
	writable bool
	appends  bool
	eof      bool
	closed   bool
}

// Read copies the bytes at the position and reports io.EOF for a read that
// finds none, the read that makes feof answer true.
func (m *memoryStream) Read(p []byte) (int, error) {
	if m.closed {
		return 0, errStreamClosed
	}
	if m.pos >= int64(len(m.buf)) {
		m.eof = true
		return 0, io.EOF
	}
	n := copy(p, m.buf[m.pos:])
	m.pos += int64(n)
	return n, nil
}

// Write stores p at the position, extending the buffer and zero-filling a gap
// a seek past the end left, as php's write does. An append mode writes at the
// end wherever the handle sits.
func (m *memoryStream) Write(p []byte) (int, error) {
	switch {
	case m.closed:
		return 0, errStreamClosed
	case !m.writable:
		return 0, errStreamReadOnly
	}
	if m.appends {
		m.pos = int64(len(m.buf))
	}
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		grown := make([]byte, end)
		copy(grown, m.buf)
		m.buf = grown
	}
	copy(m.buf[m.pos:], p)
	m.pos, m.eof = end, false
	return len(p), nil
}

// Seek moves the handle and clears the end a read reported. A position before
// the start is an error, where one past the end is where php pads from.
func (m *memoryStream) Seek(offset int64, whence int) (int64, error) {
	if m.closed {
		return 0, errStreamClosed
	}
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = m.pos + offset
	case io.SeekEnd:
		next = int64(len(m.buf)) + offset
	default:
		return 0, errors.New("fseek: unknown whence")
	}
	if next < 0 {
		return 0, errors.New("fseek: a position before the start of the stream")
	}
	m.pos, m.eof = next, false
	return next, nil
}

// AtEOF reports php's own end of file: the read that came back empty sets it,
// where a seek clears the flag and the position alone says nothing.
func (m *memoryStream) AtEOF() bool { return m.eof }

// Close drops the buffer and answers for fclose. A later call reports the
// handle as closed.
func (m *memoryStream) Close() error {
	m.buf, m.pos, m.closed = nil, 0, true
	return nil
}
