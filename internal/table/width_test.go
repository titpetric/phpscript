package table

import (
	"testing"
)

// TestWidth covers the measurement every column size rests on: plain text is
// its rune count, a colour code is nothing, and a box drawing rune is one cell.
func TestWidth(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"", 0},
		{"PASS", 4},
		{ColorGreen + "PASS" + ColorReset, 4},
		{BoxVertical + " a " + BoxVertical, 5},
	} {
		if got := Width(tc.value); got != tc.want {
			t.Errorf("Width(%q) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

// TestStrip covers the plain reading of a coloured cell. A redirected table
// and a test comparing one against literal text both read it.
func TestStrip(t *testing.T) {
	t.Parallel()
	coloured := ColorAmber + "SKIP" + ColorReset
	if got, want := Strip(coloured), "SKIP"; got != want {
		t.Errorf("Strip(%q) = %q, want %q", coloured, got, want)
	}
	if got, want := Strip("plain"), "plain"; got != want {
		t.Errorf("Strip(%q) = %q, want %q", "plain", got, want)
	}
}

// TestTruncate covers the cut a cell wider than its column takes: the result
// measures the column width, and a string already inside it is left alone.
func TestTruncate(t *testing.T) {
	t.Parallel()
	if got, want := Truncate("abcdef", 3), "abc"; got != want {
		t.Errorf("Truncate(abcdef, 3) = %q, want %q", got, want)
	}
	if got, want := Truncate("ab", 5), "ab"; got != want {
		t.Errorf("Truncate(ab, 5) = %q, want %q", got, want)
	}
	if got := Width(Truncate(ColorRed+"abcdef"+ColorReset, 4)); got != 4 {
		t.Errorf("a coloured cell cut to 4 measures %d, want 4", got)
	}
}
