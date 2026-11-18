package table

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestIsTerminal covers the choice between the box-drawing table and markdown
// when the caller has not said which it wants. A writer that is not an
// *os.File cannot be a tty, nor can a file on disk. A redirected stdout renders
// as markdown; a report file does too.
func TestIsTerminal(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	if IsTerminal(&buffer) {
		t.Error("IsTerminal(*bytes.Buffer) = true, want false for a writer that is no file")
	}

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if IsTerminal(devNull) {
		t.Errorf("IsTerminal(%s) = true, want false", os.DevNull)
	}

	report, err := os.Create(filepath.Join(t.TempDir(), "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()
	if IsTerminal(report) {
		t.Error("IsTerminal(a file on disk) = true, want false")
	}
}

// TestBoxDrawing covers the two sets of constants a table draws with: every
// box character takes one column; a border built by repeating one lines up
// with the cells above. Every color is a 256-color code ending with the reset
// sequence.
func TestBoxDrawing(t *testing.T) {
	t.Parallel()
	for _, piece := range []string{
		BoxTopLeft, BoxTopRight, BoxBottomLeft, BoxBottomRight,
		BoxHorizontal, BoxVertical, BoxTeeDown, BoxTeeUp,
		BoxTeeRight, BoxTeeLeft, BoxCross,
	} {
		if runes := len([]rune(piece)); runes != 1 {
			t.Errorf("box piece %q is %d runes, want 1", piece, runes)
		}
	}

	for _, color := range []string{
		ColorSeparator, ColorHeader, ColorAmber,
		ColorGreen, ColorWhite, ColorRed, ColorDim,
	} {
		if got, want := color[:7], "\033[38;5;"; got != want {
			t.Errorf("color %q opens with %q, want the 256-color prefix %q", color, got, want)
		}
		if color == ColorReset {
			t.Errorf("color %q is the reset sequence", color)
		}
	}

	if ColorReset != "\033[0m" {
		t.Errorf("ColorReset = %q, want the sgr reset", ColorReset)
	}
}
