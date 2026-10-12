package table

import (
	"github.com/charmbracelet/x/ansi"
)

// Width is how many terminal cells a string takes, with the escape sequences
// in it counted as nothing and a wide rune counted as two.
//
// Every column width this package computes is measured with it, and so is
// every caller that lines a table up against one this package rendered: a
// coloured cell measured with len() is as wide as its colour code, and the
// column it sits in would be padded to the wrong width.
func Width(value string) int {
	return ansi.StringWidth(value)
}

// Strip removes the escape sequences, which is what a plain text reader of a
// rendered table sees and what a test asserting on one compares.
func Strip(value string) string {
	return ansi.Strip(value)
}

// Truncate cuts a string to width terminal cells, counting as Width counts.
// The sequences ahead of the cut are kept, so a truncated cell is not left
// holding a colour nothing closes.
func Truncate(value string, width int) string {
	return ansi.Truncate(value, width, "")
}
