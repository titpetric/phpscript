// Package table renders a result table as an ansi box-drawing table on a
// terminal and as markdown everywhere else. The style constants are shared so
// every command that prints a table looks like the same program.
package table

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Box drawing characters of the ansi table.
const (
	BoxTopLeft     = "╭"
	BoxTopRight    = "╮"
	BoxBottomLeft  = "╰"
	BoxBottomRight = "╯"
	BoxHorizontal  = "─"
	BoxVertical    = "│"
	BoxTeeDown     = "┬"
	BoxTeeUp       = "┴"
	BoxTeeRight    = "├"
	BoxTeeLeft     = "┤"
	BoxCross       = "┼"
)

// Colors of the ansi table. They are 256-color codes rather than the basic
// eight, so a row keeps the same shade whatever palette the terminal ships.
const (
	ColorReset     = "\033[0m"
	ColorSeparator = "\033[38;5;238m"
	ColorHeader    = "\033[38;5;146m"
	ColorAmber     = "\033[38;5;214m"
	ColorGreen     = "\033[38;5;114m"
	ColorWhite     = "\033[38;5;255m"
	ColorRed       = "\033[38;5;167m"
	ColorDim       = "\033[38;5;244m"
)

// IsTerminal reports whether w is a tty, which is what decides between the
// box-drawing table and markdown when the caller has not said which it wants.
func IsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
