package table

import (
	"github.com/charmbracelet/x/ansi"
)

// Width is how many terminal cells a string takes, with the escape sequences
// in it counted as nothing and a wide rune counted as two.
//
// Every column width this package computes comes from it, and so does every
// caller lining a table up against one this package rendered: len() counts a
// coloured cell as wide as its colour code, and the column holding it would
// come out padded to the wrong width.
func Width(value string) int {
	return ansi.StringWidth(value)
}

// Strip removes the escape sequences. A plain text reader of a rendered table
// reads what it returns, and a test asserting on one compares against it.
func Strip(value string) string {
	return ansi.Strip(value)
}

// Truncate cuts a string to width terminal cells, counting as Width counts.
// It keeps the sequences ahead of the cut, so a truncated cell never holds a
// colour nothing closes.
func Truncate(value string, width int) string {
	return ansi.Truncate(value, width, "")
}
