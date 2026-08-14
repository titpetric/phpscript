package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsMarkdownResultsAndSummary(t *testing.T) {
	dir := t.TempDir()
	writeLintFile(t, dir, "fail.php", `<?php class Foo extends Bar {}`)
	writeLintFile(t, dir, "pass.php", `<?php echo "ok";`)
	writeLintFile(t, dir, "warn.php", `<?php if ($value = true) {}`)

	var out bytes.Buffer
	err := run([]string{dir}, false, &out)
	if err == nil {
		t.Fatal("run returned nil error with a failing file")
	}

	got := out.String()
	for _, want := range []string{
		"| Status | File | Line | Message |",
		"| FAIL | " + filepath.Join(dir, "fail.php") + " | 1 | parse error:",
		"| PASS | " + filepath.Join(dir, "pass.php") + " |  | No lint findings |",
		"| WARN | " + filepath.Join(dir, "warn.php") + " | 1 | assignment in conditional statement |",
		"Passing 1, with 1 warnings, 1 failing",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}

func TestRunFlatstackUsesOneResultPerFile(t *testing.T) {
	dir := t.TempDir()
	writeLintFile(t, dir, "pass.php", `<?php echo "ok";`)
	writeLintFile(t, dir, "warn.php", `<?php if ($value = true) {}`)

	var out bytes.Buffer
	if err := run([]string{dir}, true, &out); err != nil {
		t.Fatalf("run returned an error: %v", err)
	}

	got := out.String()
	if strings.Count(got, "| PASS |") != 1 {
		t.Errorf("got duplicate or missing passing rows:\n%s", got)
	}
	if !strings.Contains(got, "[flatstack compatible]") {
		t.Errorf("flatstack passing result is missing:\n%s", got)
	}
	if !strings.Contains(got, "Passing 1, with 1 warnings, 0 failing") {
		t.Errorf("unexpected summary:\n%s", got)
	}
}

func writeLintFile(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}
