package formatter

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// write puts src in a temporary directory and returns the path, so each case
// formats a file of its own and no case can see another one's rewrite.
func write(t *testing.T, name, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFile covers the rewrite and what it reports: a file nothing
// formatted is written back and reported as changed, and a file already
// formatted is reported unchanged and left alone. A caller running this over a
// tree counts the second result to decide whether anything moved.
func TestFile(t *testing.T) {
	t.Parallel()
	path := write(t, "index.php", "<?php\nif($a){echo 1;}\n")

	changed, err := File(path)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if !changed {
		t.Error("File reported no change for a file that was not formatted")
	}

	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Source("<?php\nif($a){echo 1;}\n")
	if err != nil {
		t.Fatal(err)
	}
	if string(formatted) != want {
		t.Errorf("the file holds %q, want %q", formatted, want)
	}

	// A second pass over the same file changes nothing, so a formatter run in
	// a pipeline reports a clean tree on the run after the one that fixed it.
	changed, err = File(path)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if changed {
		t.Error("File reported a change on a file it had already formatted")
	}
}

// TestFileSkipsWhatItCannotParse covers the refusal: formatting rewrites a
// file in place, so a file the parser cannot read is left byte for byte alone
// and reported as a *SkipError naming it. A directory of PHP holds valid code
// this parser does not support, and one such file does not stop the rest.
func TestFileSkipsWhatItCannotParse(t *testing.T) {
	t.Parallel()
	source := "<?php\nfunction broken( {\n"
	path := write(t, "broken.php", source)

	changed, err := File(path)
	if changed {
		t.Error("File reported a change for a file it refused")
	}

	var skip *SkipError
	if !errors.As(err, &skip) {
		t.Fatalf("File returned %v, want a *SkipError", err)
	}
	if skip.Path != path {
		t.Errorf("the error names %q, want %q", skip.Path, path)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != source {
		t.Errorf("the refused file now holds %q, want the source untouched", after)
	}
}

// TestFileReportsAMissingPath covers a path that is not there: the read error
// is returned as it came, with no *SkipError wrapper, because a caller walking
// a tree has to stop on a read failure and carry on past a refusal.
func TestFileReportsAMissingPath(t *testing.T) {
	t.Parallel()
	changed, err := File(filepath.Join(t.TempDir(), "absent.php"))
	if changed {
		t.Error("File reported a change for a path that is not there")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("File returned %v, want a missing file error", err)
	}

	var skip *SkipError
	if errors.As(err, &skip) {
		t.Error("a read failure was reported as a skipped file")
	}
}
