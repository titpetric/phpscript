package table

import (
	"github.com/charmbracelet/x/ansi"
)

// Width returns the terminal cells a string takes, a wide rune as two.
func Width(value string) int {
	return ansi.StringWidth(value)
}

// Strip removes the escape sequences.
func Strip(value string) string {
	return ansi.Strip(value)
}

// Truncate cuts a string to width terminal cells, keeping the sequences.
func Truncate(value string, width int) string {
	return ansi.Truncate(value, width, "")
}
