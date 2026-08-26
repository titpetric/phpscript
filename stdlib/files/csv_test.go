package files_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFPutCSVOutputStream pins the php://output handle: it writes through the
// runtime's output seam, so a buffer ob_start() pushes captures it even when
// the handle was opened first, and fclose() leaves the stream usable.
func TestFPutCSVOutputStream(t *testing.T) {
	out := runFS(t, t.TempDir(), nil, `<?php
$h = fopen("php://output", "w");
ob_start();
fputcsv($h, array("a", "b,c"));
$captured = ob_get_clean();
echo "captured:" . $captured;
fputcsv($h, array("direct"));
fclose($h);
echo "after-close";`)

	want := "captured:a,\"b,c\"\ndirect\nafter-close"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestFGetCSVKeepsThePosition pins the property the byteReader buys: a record
// read consumes exactly the record, so the rest of the file is still there for
// whatever reads next.
func TestFGetCSVKeepsThePosition(t *testing.T) {
	root := t.TempDir()
	content := "first,\"multi\nline\"\nsecond,row\ntail"
	if err := os.WriteFile(filepath.Join(root, "data.csv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
$h = fopen("data.csv", "r");
$row = fgetcsv($h);
echo count($row) . ":" . implode("|", $row) . ";";
echo stream_get_contents($h);
fclose($h);`)

	want := "2:first|multi\nline;second,row\ntail"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestFGetCSVLoopEndsFalse covers the read loop every import script writes:
// each call returns one record, and end of file answers false.
func TestFGetCSVLoopEndsFalse(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.csv"), []byte("a;b\nc;d\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
$h = fopen("data.csv", "r");
while (($row = fgetcsv($h, 0, ";")) !== false) {
	echo implode("|", $row) . ";";
}
fclose($h);
echo "done";`)

	if want := "a|b;c|d;done"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestCSVRefusesAForeignEnclosure pins the honest edge of the binding: the
// enclosure encoding/csv cannot vary is refused with a throw rather than
// producing misquoted output.
func TestCSVRefusesAForeignEnclosure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.csv"), []byte("a,b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
$w = fopen("php://output", "w");
try {
	fputcsv($w, array("a"), ",", "'");
	echo "written";
} catch (Exception $e) {
	echo "refused";
}
$r = fopen("data.csv", "r");
try {
	fgetcsv($r, 0, ",", "'");
	echo "|read";
} catch (Exception $e) {
	echo "|refused";
}
fclose($r);`)

	if want := "refused|refused"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestFOpenUnimplementedWrapper pins that a php:// name is a stream or false,
// never a file: no php:/input appears on disk.
func TestFOpenUnimplementedWrapper(t *testing.T) {
	root := t.TempDir()
	out := runFS(t, root, nil, `<?php
var_dump(fopen("php://input", "r"));`)
	if want := "bool(false)\n"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("fopen created %v in the root", entries)
	}
}
