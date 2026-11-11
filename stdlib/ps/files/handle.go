package files

import (
	"io"
	"os"
)

// fileStream is the handle fopen() answers with for a file: an *os.File that
// holds php's end-of-file flag. A position answers for no flag of php's own:
// php raises it on the read that ran out of bytes, then a seek lowers it.
type fileStream struct {
	*os.File
	regular bool
	eof     bool
}

// openFile opens name in a php fopen mode and wraps the handle.
func openFile(name, mode string) (*fileStream, error) {
	f, err := openMode(name, mode)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	return &fileStream{File: f, regular: err == nil && info.Mode().IsRegular()}, nil
}

// Read reads through to the file, raising the end on a read that came back
// with nothing. A read short of its buffer ends a regular file, as php's fill
// loop reads twice there; on a pipe the bytes that arrived are no end.
func (f *fileStream) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if n == 0 || (f.regular && n < len(p)) {
		f.eof = true
	}
	return n, err
}

// Seek moves the handle and lowers the end, as php's seek does.
func (f *fileStream) Seek(offset int64, whence int) (int64, error) {
	pos, err := f.File.Seek(offset, whence)
	if err == nil {
		f.eof = false
	}
	return pos, err
}

// AtEOF reports php's end of file: the read that ran out raised it.
func (f *fileStream) AtEOF() bool { return f.eof }

// atEOF reads the end off a handle keeping php's own flag, and reports whether
// one answered. A handle with no flag is open, php's answer for a write handle.
func atEOF(stream any) bool {
	r, ok := stream.(interface{ AtEOF() bool })
	return ok && r.AtEOF()
}

var _ io.ReadWriteSeeker = (*fileStream)(nil)
