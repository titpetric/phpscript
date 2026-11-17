package pexec

import (
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerEscape installs the two quoting functions, which need no root.
func registerEscape(rt *runner.Runtime) {
	// escapeshellarg returns $arg single-quoted for the shell, with embedded single quotes escaped.
	rt.RegisterFunc("escapeshellarg", phpEscapeshellarg)
	// escapeshellcmd returns $command with the shell metacharacters in it
	// backslash-escaped, leaving quotes that come in pairs alone;
	// escapeshellarg is the one to call, since this leaves a quoted
	// argument's own contents live.
	rt.RegisterFunc("escapeshellcmd", phpEscapeshellcmd)
}

// phpEscapeshellarg wraps an argument in single quotes and escapes an embedded quote.
func phpEscapeshellarg(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// shellMeta are the bytes escapeshellcmd escapes, the ones a shell reads as syntax.
const shellMeta = "#&;`|*?~<>^()[]{}$\\\x0a\xff"

// phpEscapeshellcmd escapes shell metacharacters, leaving a paired quote and its contents live.
func phpEscapeshellcmd(command string) string {
	var b strings.Builder
	b.Grow(len(command))
	// pending is where the open quote closes, or -1
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
