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
	// escapeshellarg is the call to use, since this leaves a quoted
	// argument's contents live.
	rt.RegisterFunc("escapeshellcmd", phpEscapeshellcmd)
}

// phpEscapeshellarg wraps an argument in single quotes and escapes an embedded quote.
func phpEscapeshellarg(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// shellMeta lists the bytes escapeshellcmd escapes, that a shell reads as syntax.
const shellMeta = "#&;`|*?~<>^()[]{}$\\\x0a\xff"

// phpEscapeshellcmd escapes shell metacharacters, leaving a paired quote and its contents live.
func phpEscapeshellcmd(command string) string {
	var b strings.Builder
	b.Grow(len(command))
	// pending names where the open quote closes, or -1 when no quote is open.
	// Opening one looks ahead for its partner and leaves both alone. A quote
	// without a partner, or one of the other kind found while a quote is open,
	// is a stray and gets escaped.
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
