package version

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestInfo covers the sentinel the four fields carry. A build with no ldflags
// behind it fills Commit, CommitTime and Branch with "unknown", and a line
// reading "Commit: unknown" is noise and Run prints each of the three only
// when it holds something else. The version line is always printed, because
// "dev" is itself the report of an unreleased binary.
func TestInfo(t *testing.T) {
	out := render(t, Info{Version: "dev", Commit: "unknown", CommitTime: "unknown", Branch: "unknown"})

	if !strings.Contains(out, "  Version:     dev\n") {
		t.Errorf("the version line is missing:\n%s", out)
	}
	for _, label := range []string{"  Commit:", "  CommitTime:", "  Branch:"} {
		if strings.Contains(out, label) {
			t.Errorf("%q is printed for an unknown value:\n%s", label, out)
		}
	}
}

// TestRun covers the VCS block an injected build prints. Run cuts the commit
// to twelve characters, the width of a git short hash and the column does not
// carry forty characters nobody compares by eye. The full hash stays in the
// Info; the cut touches the printed form alone.
func TestRun(t *testing.T) {
	out := render(t, Info{
		Version:    "1.4.0",
		Commit:     "4d681de0a1b2c3d4e5f60718293a4b5c6d7e8f90",
		CommitTime: "2026-10-09T11:02:17Z",
		Branch:     "main",
	})

	for _, want := range []string{
		"  Version:     1.4.0\n",
		"  Commit:      4d681de0a1b2\n",
		"  CommitTime:  2026-10-09T11:02:17Z\n",
		"  Branch:      main\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "4d681de0a1b2c") {
		t.Errorf("the commit was printed past twelve characters:\n%s", out)
	}

	// Run prints a short commit whole: the cut is a ceiling and no padding.
	short := render(t, Info{Version: "1.4.0", Commit: "4d681de", CommitTime: "unknown", Branch: "unknown"})
	if !strings.Contains(short, "  Commit:      4d681de\n") {
		t.Errorf("a short commit was not printed whole:\n%s", short)
	}
}

// TestNewCommand covers where the command reads its figures from. Build is a
// package variable main fills before it builds any command and the Run closure
// reads it at call time and takes no argument for it: build information
// describes the binary, and neither the configuration nor a flag carries it.
func TestNewCommand(t *testing.T) {
	command := NewCommand(nil, nil)

	if command.Name != "version" {
		t.Errorf("Name = %q, want %q", command.Name, "version")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind != nil {
		t.Error("Bind is set, want nil: the version command declares no flags")
	}

	saved := Build
	t.Cleanup(func() { Build = saved })
	Build = Info{Version: "9.9.9", Commit: "unknown", CommitTime: "unknown", Branch: "unknown"}

	out := captureStdout(t, func() {
		if err := command.Run(t.Context(), nil); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "  Version:     9.9.9\n") {
		t.Errorf("the command did not print the recorded build:\n%s", out)
	}
}

// render runs Run over one Info and returns what it printed.
func render(t *testing.T, info Info) string {
	t.Helper()

	return captureStdout(t, func() {
		if err := Run(info); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}

// captureStdout runs fn with os.Stdout on a pipe and returns what it wrote.
// Run prints through the package-level fmt and the file descriptor is the only
// place to read its output from.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	saved := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	fn()
	writer.Close()
	os.Stdout = saved

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
