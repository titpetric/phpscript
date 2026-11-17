package formatter

import (
	"strings"
)

// styleScan rewrites go runtime source text and tracks quotes and comments across lines.
type styleScan struct {
	blockComment bool
	quote        byte
}

// Keep is what a semicolon ending a declaration line would break.
var keepSemicolon = map[string]bool{
	"function": true, "abstract": true, "interface": true, "class": true,
	"namespace": true, "public": true, "private": true, "protected": true,
	"static": true, "var": true, "readonly": true, "do": true, "while": true,
	"for": true, "foreach": true, "if": true, "else": true, "elseif": true,
	"switch": true, "case": true, "catch": true, "declare": true, "try": true,
	"finally": true,
}

// keepsSemicolon reports whether a line's trailing semicolon is part of its syntax.
func keepsSemicolon(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "} while") {
		return true
	}
	return keepSemicolon[firstWord(trimmed)]
}

// isGoRuntimeSource reports whether a template file writes go runtime syntax.
func isGoRuntimeSource(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `import "`) || strings.HasPrefix(strings.ToLower(line), "use runtime\\go") {
			return true
		}
	}
	return false
}

// line rewrites one line of go runtime source.
func (s *styleScan) line(line string) string {
	if line == "" {
		return line
	}
	out := make([]byte, 0, len(line))
	lastSemi := -1
	afterSemi := false
	lineComment := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if s.blockComment {
			out = append(out, c)
			if c == '*' && i+1 < len(line) && line[i+1] == '/' {
				out = append(out, '/')
				i++
				s.blockComment = false
			}
			continue
		}
		if s.quote != 0 {
			out = append(out, c)
			if c == '\\' && i+1 < len(line) {
				out = append(out, line[i+1])
				i++
				continue
			}
			if c == s.quote {
				s.quote = 0
			}
			continue
		}
		if lineComment {
			out = append(out, c)
			continue
		}
		switch {
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			out = append(out, c)
			lineComment = true
		case c == '#':
			out = append(out, c)
			lineComment = true
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			out = append(out, c, line[i+1])
			i++
			s.blockComment = true
		case c == '\'' || c == '"':
			out = append(out, c)
			s.quote = c
		case c == '-' && i+1 < len(line) && line[i+1] == '>':
			out = append(out, '-', '>')
			i++
			start := i + 1
			end := start
			for end < len(line) && isMemberByte(line[end]) {
				end++
			}
			out = append(out, lowerMember(line[start:end])...)
			i = end - 1
		case c == ';':
			out = append(out, c)
			lastSemi = len(out) - 1
			afterSemi = false
		default:
			out = append(out, c)
			if c != ' ' && c != '\t' && lastSemi >= 0 {
				afterSemi = true
			}
		}
	}
	if lastSemi >= 0 && !afterSemi && !keepsSemicolon(string(out)) {
		out = append(out[:lastSemi], out[lastSemi+1:]...)
	}
	return string(out)
}

// isMemberByte reports whether c continues a member name.
func isMemberByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// lowerMember spells a go name in lowerCamelCase, turning an initialism down whole.
func lowerMember(name string) string {
	run := 0
	for run < len(name) && name[run] >= 'A' && name[run] <= 'Z' {
		run++
	}
	if run == 0 {
		return name
	}
	if run == len(name) {
		return strings.ToLower(name)
	}
	if run > 1 {
		run--
	}
	return strings.ToLower(name[:run]) + name[run:]
}

// firstWord answers the first token of a trimmed line.
func firstWord(line string) string {
	line = strings.TrimSpace(line)
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t', '(', '$', '{', '}', ';', ':', '=':
			if i > 0 {
				return line[:i]
			}
			return line[:i+1]
		}
	}
	return line
}
