package files

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestMemoryMode covers every fopen mode php accepts, as php's own wrapper
// answers them: "r", "x" and "c" refuse a write, a "+" on any of the three
// takes one, where an append mode writes at the end.
func TestMemoryMode(t *testing.T) {
	t.Parallel()
	for mode, want := range map[string][2]bool{
		"r": {false, false}, "rb": {false, false}, "r+": {true, false},
		"w": {true, false}, "w+": {true, false}, "wb": {true, false},
		"a": {true, true}, "a+": {true, true}, "ab": {true, true},
		"x": {false, false}, "x+": {true, false},
		"c": {false, false}, "c+": {true, false},
	} {
		writable, appends := memoryMode(mode)
		if got := [2]bool{writable, appends}; got != want {
			t.Errorf("memoryMode(%q) = %v, want %v", mode, got, want)
		}
	}
}

// TestMemoryStreamAppends covers the write an append mode moves: php writes at
// the end wherever the handle sits, where every other mode writes in place.
func TestMemoryStreamAppends(t *testing.T) {
	t.Parallel()
	m := &memoryStream{writable: true, appends: true}
	if _, err := m.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Write([]byte("X")); err != nil {
		t.Fatal(err)
	}
	if want := "abcX"; string(m.buf) != want {
		t.Errorf("buffer = %q, want %q", string(m.buf), want)
	}
}

// TestStreamBounds covers the optional arguments stream_get_contents takes as
// php's own numbers arrive: an int64, a float, a null, a numeric string.
func TestStreamBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		args           []any
		length, offset int64
	}{
		{"absent", nil, -1, -1},
		{"length", []any{int64(3)}, 3, -1},
		{"both", []any{int64(3), int64(1)}, 3, 1},
		{"nil", []any{nil, nil}, -1, -1},
		{"float", []any{float64(4), float64(2)}, 4, 2},
		{"int", []any{5, 2}, 5, 2},
		{"numeric string", []any{"3", "1"}, 3, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			length, offset := streamBounds(tc.args)
			if length != tc.length || offset != tc.offset {
				t.Errorf("streamBounds(%v) = %d, %d, want %d, %d", tc.args, length, offset, tc.length, tc.offset)
			}
		})
	}
}

// TestMemoryStreamPadsAGap covers the write php pads: a seek past the end
// leaves NUL bytes behind, which no fixture can spell in its expected output.
func TestMemoryStreamPadsAGap(t *testing.T) {
	t.Parallel()
	m := &memoryStream{writable: true}
	if _, err := m.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Write([]byte("z")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(m)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ab\x00\x00\x00z"; string(b) != want {
		t.Errorf("buffer = %q, want %q", string(b), want)
	}
}

// TestMemoryStreamRefusesAWrite covers a handle opened for reading: the write
// fails, which fwrite reports as false.
func TestMemoryStreamRefusesAWrite(t *testing.T) {
	t.Parallel()
	m := &memoryStream{}
	if _, err := m.Write([]byte("x")); !errors.Is(err, errStreamReadOnly) {
		t.Errorf("err = %v, want %v", err, errStreamReadOnly)
	}
}

// TestMemoryStreamAfterClose covers the calls php answers with a TypeError:
// each one fails here, reporting the handle as closed.
func TestMemoryStreamAfterClose(t *testing.T) {
	t.Parallel()
	m := &memoryStream{writable: true}
	if _, err := m.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Read(make([]byte, 3)); !errors.Is(err, errStreamClosed) {
		t.Errorf("read err = %v, want %v", err, errStreamClosed)
	}
	if _, err := m.Write([]byte("x")); !errors.Is(err, errStreamClosed) {
		t.Errorf("write err = %v, want %v", err, errStreamClosed)
	}
	if _, err := m.Seek(0, io.SeekStart); !errors.Is(err, errStreamClosed) {
		t.Errorf("seek err = %v, want %v", err, errStreamClosed)
	}
}

// TestMemoryStreamSeekRefusals covers the seeks php rejects: a position before
// the start of the buffer, a whence outside SEEK_SET, SEEK_CUR, SEEK_END.
func TestMemoryStreamSeekRefusals(t *testing.T) {
	t.Parallel()
	m := &memoryStream{writable: true}
	if _, err := m.Seek(-1, io.SeekStart); err == nil {
		t.Error("a seek before the start must fail")
	}
	if _, err := m.Seek(0, 7); err == nil {
		t.Error("an unknown whence must fail")
	}
}

// TestOpenMemoryTakesAnArgument covers the suffix php's temp scheme accepts:
// `temp/maxmemory:1024` opens the same buffer the bare name opens, where
// `memory/foo` is a name php refuses and fopen answers false for.
func TestOpenMemoryTakesAnArgument(t *testing.T) {
	t.Parallel()
	rt := runner.New(os.Stdout, runner.Options{})
	for _, scheme := range []string{"memory", "temp", "temp/maxmemory:1024", "TEMP/maxmemory:8"} {
		handle, ok, err := openWrapper(rt, scheme, "w+")
		if !ok || err != nil {
			t.Fatalf("openWrapper(%q) = %v, ok %v, err %v", scheme, handle, ok, err)
		}
		if _, isMemory := handle.(*memoryStream); !isMemory {
			t.Errorf("openWrapper(%q) handed back %T", scheme, handle)
		}
	}
	for _, scheme := range []string{"memory/foo", "temp/nonsense", "memory/maxmemory:8"} {
		if handle, ok, err := openWrapper(rt, scheme, "w+"); ok || handle != nil || err != nil {
			t.Errorf("openWrapper(%q) = %v, ok %v, err %v", scheme, handle, ok, err)
		}
	}
}
