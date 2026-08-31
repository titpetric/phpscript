package pexec

import (
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerEscape installs the two quoting functions. They touch nothing and are
// not bound to a root: quoting is a string operation, and a script quotes an
// argument whether or not it goes on to run anything.
func registerEscape(rt *runner.Runtime) {
	// escapeshellarg returns $arg single-quoted for the shell, with embedded single quotes escaped.
	rt.RegisterFunc("escapeshellarg", phpEscapeshellarg)
	// escapeshellcmd returns $command with the shell metacharacters in it backslash-escaped, leaving quotes that come in pairs alone; escapeshellarg is the one to reach for, since this leaves a quoted argument's own contents live.
	rt.RegisterFunc("escapeshellcmd", phpEscapeshellcmd)
}

// phpEscapeshellarg wraps an argument in single quotes, where nothing but a
// single quote is special, and splices the quote itself out of the string and
// back in escaped.
func phpEscapeshellarg(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// shellMeta are the bytes escapeshellcmd escapes. They are the ones a shell
// reads as syntax rather than as text.
const shellMeta = "#&;`|*?~<>^()[]{}$\\\x0a\xff"

// phpEscapeshellcmd escapes the shell metacharacters in a whole command line.
//
// A quote is escaped only when it is unpaired, which is what lets a caller write
// escapeshellcmd("echo 'a b'") and keep the quoting they meant. That also means
// the contents of a quoted argument stay live, so this is not the function to
// pass user input through; escapeshellarg is.
func phpEscapeshellcmd(command string) string {
	var b strings.Builder
	b.Grow(len(command))
	// pending is where the quote currently open is expected to close, or -1
	// when none is open. Opening one looks ahead for its partner and leaves
	// both alone; a quote with no partner, and one of the other kind found
	// while a quote is open, is a stray and gets escaped.
	pending := -1
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch c {
		case '"', '\'':
			switch {
			case pending < 0:
				at := strings.IndexByte(command[i+1:], c)
				if at < 0 {
					b.WriteByte('\\')
					break
				}
				pending = i + 1 + at
			case command[pending] == c:
				pending = -1
			default:
				b.WriteByte('\\')
			}
		default:
			if strings.IndexByte(shellMeta, c) >= 0 {
				b.WriteByte('\\')
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}
