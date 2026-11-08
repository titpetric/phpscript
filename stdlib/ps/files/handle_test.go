package files

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// fileHandle opens a 15-byte file through openFile, the way fopen does.
func fileHandle(t *testing.T, mode string) *fileStream {
	t.Helper()
	name := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(name, []byte("line1\nline2\nend"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := openFile(name, mode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// TestFileStreamEndsOnAShortRead covers php's end of file on a file: a read
// that came back short of the buffer sets it, where a read that filled the
// buffer exactly leaves the handle open.
func TestFileStreamEndsOnAShortRead(t *testing.T) {
	t.Parallel()
	f := fileHandle(t, "r")
	if n, err := f.Read(make([]byte, 15)); n != 15 || err != nil {
		t.Fatalf("read = %d, %v", n, err)
	}
	if f.AtEOF() {
		t.Error("a read that filled the buffer must leave the handle open")
	}
	if n, err := f.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatalf("read = %d, %v", n, err)
	}
	if !f.AtEOF() {
		t.Error("a read that came back empty must report the end")
	}

	g := fileHandle(t, "r")
	if n, err := g.Read(make([]byte, 100)); n != 15 || err != nil {
		t.Fatalf("read = %d, %v", n, err)
	}
	if !g.AtEOF() {
		t.Error("a short read must report the end")
	}
}

// TestFileStreamSeekClearsTheEnd covers the seek php's own clears: a position
// at the end reports nothing by itself, and a seek after a read past the end
// opens the handle again.
func TestFileStreamSeekClearsTheEnd(t *testing.T) {
	t.Parallel()
	f := fileHandle(t, "r")
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	if f.AtEOF() {
		t.Error("a seek to the end must leave the handle open")
	}
	if _, err := f.Read(make([]byte, 5)); err != io.EOF {
		t.Fatalf("read err = %v, want %v", err, io.EOF)
	}
	if !f.AtEOF() {
		t.Fatal("the read past the end must report it")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if f.AtEOF() {
		t.Error("a seek must clear the end")
	}
}

// TestFileStreamWriteKeepsTheHandleOpen covers a handle a script only writes:
// php answers false for every feof on one, the write included.
func TestFileStreamWriteKeepsTheHandleOpen(t *testing.T) {
	t.Parallel()
	f := fileHandle(t, "a")
	if _, err := f.Write([]byte("!")); err != nil {
		t.Fatal(err)
	}
	if f.AtEOF() {
		t.Error("a write must leave the handle open")
	}
}

// TestFileStreamOnAPipe covers the short read a pipe answers with: the bytes
// available are not the end, where a read that came back empty is.
func TestFileStreamOnAPipe(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if _, err := w.WriteString("ab"); err != nil {
		t.Fatal(err)
	}

	pipe := &fileStream{File: r}
	if n, err := pipe.Read(make([]byte, 100)); n != 2 || err != nil {
		t.Fatalf("read = %d, %v", n, err)
	}
	if pipe.AtEOF() {
		t.Error("a short read of a pipe must leave the handle open")
	}
	w.Close()
	if n, err := pipe.Read(make([]byte, 100)); n != 0 || err != io.EOF {
		t.Fatalf("read = %d, %v", n, err)
	}
	if !pipe.AtEOF() {
		t.Error("a read that came back empty must report the end")
	}
}
