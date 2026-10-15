package files_test

import (
	"os"
	"path/filepath"
	"testing"
)

// streamRoot builds a project holding one three-line file, the shape a script
// walking a handle reads.
func streamRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "log.txt"), []byte("line1\nline2\nend"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestFreadAdvancesTheHandle covers the read half against what php prints:
// fread returns at most $length bytes, the handle advances by what it
// returned, and a read at the end answers with the empty string, so a loop
// reading to the end terminates.
func TestFreadAdvancesTheHandle(t *testing.T) {
	t.Parallel()
	out := runFS(t, streamRoot(t), nil, `<?php
$f = fopen("log.txt", "r");
echo "[", fread($f, 5), "][", ftell($f), "]";
echo "[", fread($f, 100), "]";
echo "[", fread($f, 5), "][", feof($f) ? "eof" : "open", "]";
fclose($f);`)
	if want := "[line1][5][\nline2\nend][][eof]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestFgetsKeepsTheNewline covers what php's fgets returns: the line up to and
// including its newline, and false once the handle is spent. A script reading
// with `while (($line = fgets($f)) !== false)` leans on the false, so an empty
// string at the end would loop forever.
func TestFgetsKeepsTheNewline(t *testing.T) {
	t.Parallel()
	out := runFS(t, streamRoot(t), nil, `<?php
$f = fopen("log.txt", "r");
while (($line = fgets($f)) !== false) {
	echo "[", $line, "]";
}
echo feof($f) ? "eof" : "open";
fclose($f);`)
	if want := "[line1\n][line2\n][end]eof"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestFseekMovesByWhence covers the three SEEK_* values and their
// constants. PHP numbers them 0, 1 and 2, and so does Go's io package, so the
// argument is passed through as it arrives; a translation table here would be a
// second numbering to keep in step.
func TestFseekMovesByWhence(t *testing.T) {
	t.Parallel()
	out := runFS(t, streamRoot(t), nil, `<?php
echo "[", SEEK_SET, ",", SEEK_CUR, ",", SEEK_END, "]";
$f = fopen("log.txt", "r");
echo "[", fseek($f, 6, SEEK_SET), ",", ftell($f), ",", fread($f, 5), "]";
echo "[", fseek($f, 1, SEEK_CUR), ",", ftell($f), "]";
echo "[", fseek($f, -3, SEEK_END), ",", fread($f, 10), "]";
fclose($f);`)
	if want := "[0,1,2][0,6,line2][0,12][0,end]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestRewindReturnsToTheStart covers rewind against ftell: the handle is back
// at offset zero and the next read is the first bytes of the file again.
func TestRewindReturnsToTheStart(t *testing.T) {
	t.Parallel()
	out := runFS(t, streamRoot(t), nil, `<?php
$f = fopen("log.txt", "r");
fread($f, 11);
echo "[", ftell($f), "]";
echo "[", rewind($f) ? "y" : "n", ",", ftell($f), ",", fread($f, 3), "]";
fclose($f);`)
	if want := "[11][y,0,lin]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestFeofComparesPositionAgainstLength covers the rule the binding states:
// feof reports where the handle sits against how long the file is, so a handle
// moved to the end with fseek answers eof without a read. php reports the end
// only once a read has run past it, so php answers false at that same offset;
// the divergence is in this direction alone and never reports open at a
// position past the end.
func TestFeofComparesPositionAgainstLength(t *testing.T) {
	t.Parallel()
	out := runFS(t, streamRoot(t), nil, `<?php
$f = fopen("log.txt", "r");
echo "[", feof($f) ? "eof" : "open", "]";
fseek($f, 0, SEEK_END);
echo "[", feof($f) ? "eof" : "open", "]";
rewind($f);
echo "[", feof($f) ? "eof" : "open", "]";
fclose($f);`)
	if want := "[open][eof][open]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestFeofOnAnUnseekableHandle covers php://input and php://output, which
// cannot be seeked: feof answers false for them, so a script draining the
// request body with a feof loop would spin, and stream_get_contents is the
// call for that.
func TestFeofOnAnUnseekableHandle(t *testing.T) {
	t.Parallel()
	out := runFS(t, streamRoot(t), nil, `<?php
echo "[", feof(fopen("php://output", "w")) ? "eof" : "open", "]";`)
	if want := "[open]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
