package run

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveEntrypoint pins which scripts re-root the runtime. A path the
// working directory can name keeps it as the root, because PHP resolves an
// include against the working directory; anything the working directory cannot
// name has to root at the script instead, or the leading slash is stripped and
// the file is looked for in the wrong place.
func TestResolveEntrypoint(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	outside := filepath.Join(t.TempDir(), "other.php")

	cases := []struct {
		name       string
		arg        string
		wantScript string
		wantRoot   string
	}{
		{"relative", "sub/x.php", "sub/x.php", "."},
		{"relative dot", "./x.php", "x.php", "."},
		{"absolute inside", filepath.Join(dir, "x.php"), "x.php", "."},
		{"absolute outside", outside, "other.php", filepath.Dir(outside)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script, root, err := resolveEntrypoint(c.arg)
			if err != nil {
				t.Fatalf("resolveEntrypoint(%q): %v", c.arg, err)
			}
			if script != c.wantScript || root != c.wantRoot {
				t.Fatalf("resolveEntrypoint(%q) = %q, %q, want %q, %q",
					c.arg, script, root, c.wantScript, c.wantRoot)
			}
		})
	}
}

// TestResolveEntrypointClimbingOut covers the sibling directory case, which
// looks relative but the working directory still cannot name it.
func TestResolveEntrypointClimbingOut(t *testing.T) {
	dir := t.TempDir()
	here := filepath.Join(dir, "here")
	there := filepath.Join(dir, "there")
	for _, d := range []string{here, there} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(here)

	script, root, err := resolveEntrypoint("../there/x.php")
	if err != nil {
		t.Fatal(err)
	}
	if script != "x.php" || root != there {
		t.Fatalf("resolveEntrypoint = %q, %q, want %q, %q", script, root, "x.php", there)
	}
}
