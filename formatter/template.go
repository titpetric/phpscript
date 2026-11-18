package formatter

import (
	"strings"
)

// This file aligns the lines of a template: a file that opens and closes PHP
// on one line. The statement printer rewrites statements. Only a template
// line's leading indentation moves. PHP lines follow the block nesting of the
// multi-line code, and markup follows the container tags around it.

// containerTags are the markup elements whose contents indent one level.
var containerTags = map[string]bool{
	"aside": true, "article": true, "body": true, "details": true, "div": true,
	"dl": true, "fieldset": true, "figure": true, "footer": true, "form": true,
	"head": true, "header": true, "main": true, "nav": true, "ol": true,
	"section": true, "select": true, "table": true, "tbody": true, "tfoot": true,
	"thead": true, "tr": true, "ul": true,
}

// phpScan tracks the comment and quote state across lines of PHP.
type phpScan struct {
	blockComment bool
	quote        byte
}

// alignTemplate re-indents src and applies the go runtime spelling to its code lines.
func alignTemplate(src string, goRuntime bool) string {
	lines := strings.Split(src, "\n")
	var tags []string
	var scan phpScan
	var style styleScan
	skip := map[int]bool{}
	base, phpNest := 0, 0
	prevCode := ""
	inPHP := false
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimLeft(line, " \t")
		switch {
		case trimmed == "":
			if goRuntime && isAssignCallSource(prevCode, nextCode(lines, i)) {
				skip[i] = true
			}
			lines[i] = ""
		case isPHPInline(trimmed):
			// A template line that is PHP alone stays at the code's depth: a
			// brace it opens nests the markup that follows.
			code := phpCode(trimmed)
			lead := leadingClosers(strings.TrimLeft(code, " \t"))
			lines[i] = indentLine(base+phpNest-lead, trimmed)
			delta, closeAt := scan.code(code)
			phpNest += delta
			if closeAt >= 0 {
				rest := trimmed[len(trimmed)-len(code)+closeAt+2:]
				tags = scanTags(stripPHP(rest), tags)
			}
		case startsPHP(trimmed):
			code := phpCode(trimmed)
			if goRuntime && strings.TrimSpace(code) != "" {
				trimmed = strings.TrimSuffix(trimmed, code) + style.line(code)
			}
			lines[i] = indentLine(base+phpNest, trimmed)
			delta, _ := scan.code(phpCode(trimmed))
			base += delta
			prevCode = trimmed
			inPHP = true
		case inPHP:
			// The PHP code between the tags keeps its own indentation; only
			// the template's lines move. The scan still reads the nesting the
			// template lines below it follow.
			if goRuntime {
				line = style.line(line)
				trimmed = strings.TrimLeft(line, " \t")
			}
			delta, closeAt := scan.code(trimmed)
			lines[i] = line
			base += delta
			prevCode = trimmed
			if closeAt >= 0 {
				inPHP = false
				tags = scanTags(stripPHP(trimmed[closeAt+2:]), tags)
			}
		default:
			html := stripPHP(trimmed)
			for {
				name, rest, ok := leadingClose(html)
				if !ok {
					break
				}
				html = rest
				if n := len(tags); n > 0 && tags[n-1] == name {
					tags = tags[:n-1]
				}
			}
			lines[i] = indentLine(base+nestDepth(phpNest, len(tags)), trimmed)
			tags = scanTags(html, tags)
		}
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if !skip[i] {
			out = append(out, line)
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// nextCode answers the next line that holds anything.
func nextCode(lines []string, i int) string {
	for j := i + 1; j < len(lines); j++ {
		if line := strings.TrimSpace(lines[j]); line != "" {
			return line
		}
	}
	return ""
}

// isAssignCallSource reports a call written directly beneath an assignment.
func isAssignCallSource(prev, next string) bool {
	if !strings.HasPrefix(prev, "$") || !strings.HasSuffix(strings.TrimSuffix(next, ";"), ")") {
		return false
	}
	if !strings.Contains(next, "->") && !strings.Contains(next, "(") {
		return false
	}
	return strings.Contains(prev, " = ") || strings.HasSuffix(prev, " =")
}

// nestDepth answers the deeper of the PHP and markup depths.
func nestDepth(phpNest, html int) int {
	if phpNest > html {
		return phpNest
	}
	return html
}

// startsPHP reports whether s opens a PHP block.
func startsPHP(s string) bool {
	return strings.HasPrefix(s, "<?php") || strings.HasPrefix(s, "<?=")
}

// isPHPInline reports whether s opens and closes PHP on a line alone.
func isPHPInline(s string) bool {
	if !startsPHP(s) {
		return false
	}
	return strings.Contains(phpCode(s), "?>")
}

// phpCode strips the opening tag from a line that starts with one.
func phpCode(s string) string {
	if rest, ok := strings.CutPrefix(s, "<?php"); ok {
		return rest
	}
	return strings.TrimPrefix(s, "<?=")
}

// indentLine replaces the leading whitespace of line with depth tabs.
func indentLine(depth int, line string) string {
	if depth < 0 {
		depth = 0
	}
	return strings.Repeat("\t", depth) + line
}

// leadingClosers counts the closing brackets a line starts with.
func leadingClosers(s string) int {
	n := 0
	for n < len(s) {
		switch s[n] {
		case '}', ']', ')':
			n++
			continue
		}
		break
	}
	return n
}

// code counts the nesting a PHP run opens, answering where `?>` ends it.
func (s *phpScan) code(src string) (delta, closeAt int) {
	closeAt = -1
	lineComment := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if s.blockComment {
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				s.blockComment = false
				i++
			}
			continue
		}
		if s.quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == s.quote {
				s.quote = 0
			}
			continue
		}
		if lineComment {
			if c == '?' && i+1 < len(src) && src[i+1] == '>' {
				return delta, i
			}
			continue
		}
		switch c {
		case '/':
			if i+1 < len(src) {
				switch src[i+1] {
				case '/':
					lineComment = true
					i++
				case '*':
					s.blockComment = true
					i++
				}
			}
		case '#':
			lineComment = true
		case '\'', '"':
			s.quote = c
		case '?':
			if i+1 < len(src) && src[i+1] == '>' {
				return delta, i
			}
		case '{', '[', '(':
			delta++
		case '}', ']', ')':
			delta--
		}
	}
	return delta, closeAt
}

// stripPHP replaces the inline PHP blocks of a line with spaces.
func stripPHP(line string) string {
	var b strings.Builder
	for {
		start := strings.Index(line, "<?")
		if start < 0 {
			b.WriteString(line)
			return b.String()
		}
		b.WriteString(line[:start])
		end := strings.Index(line[start:], "?>")
		if end < 0 {
			return b.String()
		}
		line = line[start+end+2:]
		b.WriteByte(' ')
	}
}

// leadingClose reads `</tag>` at the start of s and answers what follows it.
func leadingClose(s string) (name, rest string, ok bool) {
	trimmed := strings.TrimLeft(s, " \t")
	if !strings.HasPrefix(trimmed, "</") {
		return "", s, false
	}
	end := strings.IndexByte(trimmed, '>')
	if end < 0 {
		return "", s, false
	}
	name = tagName(trimmed[2:end])
	return name, trimmed[end+1:], name != ""
}

// scanTags applies the tags of one line to the open container stack.
func scanTags(line string, tags []string) []string {
	for i := 0; i < len(line); i++ {
		if line[i] != '<' {
			continue
		}
		if i+1 < len(line) && (line[i+1] == '!' || line[i+1] == '?') {
			continue
		}
		end := strings.IndexByte(line[i:], '>')
		if end < 0 {
			break
		}
		inner := line[i+1 : i+end]
		i += end
		if strings.HasPrefix(inner, "/") {
			name := tagName(inner[1:])
			if n := len(tags); n > 0 && tags[n-1] == name {
				tags = tags[:n-1]
			}
			continue
		}
		if strings.HasSuffix(inner, "/") {
			continue
		}
		if name := tagName(inner); containerTags[name] {
			tags = append(tags, name)
		}
	}
	return tags
}

// tagName answers the element name a tag opens with.
func tagName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t\r\n/"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}
