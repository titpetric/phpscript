package formatter

import (
	"strings"

	"github.com/titpetric/phpscript/parser"
)

// Comment is one comment of the source, with the lines it occupied. The AST
// holds no comments and the formatter reads them off the source separately and
// places them by line: a comment goes out before the first statement that
// starts below it. OwnLine records that nothing but whitespace came before it,
// which separates a comment an author wrote above a statement from one written
// after it.
type Comment struct {
	Text    string
	Line    int
	EndLine int
	OwnLine bool
}

// flushComments writes every comment above line, or the rest when line is zero.
func (p *printer) flushComments(line int) {
	for p.nextComment < len(p.comments) {
		c := p.comments[p.nextComment]
		if line > 0 && c.Line >= line {
			return
		}
		p.nextComment++
		p.comment(c)
	}
}

// trailingComment appends a comment written after the code on line to that line.
func (p *printer) trailingComment(line int) {
	if line <= 0 || p.nextComment >= len(p.comments) {
		return
	}
	c := p.comments[p.nextComment]
	if c.OwnLine || c.Line != line {
		return
	}
	p.nextComment++
	p.appendToLine(" " + c.Text)
	p.lastLine = c.EndLine
}

// appendToLine adds text to the end of the line last written.
func (p *printer) appendToLine(text string) {
	out := p.buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		p.buf.WriteString(text)
		return
	}
	p.buf.Truncate(p.buf.Len() - 1)
	p.buf.WriteString(text)
	p.buf.WriteByte('\n')
}

// blankFor writes the blank line the author left above line, none at the start of a block.
func (p *printer) blankFor(line int) {
	if p.lastLine > 0 && line > p.lastLine+1 {
		p.blank()
	}
}

// comment writes one comment at the current indent, re-indenting a block comment's asterisks.
func (p *printer) comment(c Comment) {
	p.blankFor(c.Line)
	lines := strings.Split(strings.ReplaceAll(c.Text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if i > 0 {
			if trimmed := strings.TrimLeft(line, " \t"); strings.HasPrefix(trimmed, "*") {
				line = " " + trimmed
			} else {
				line = strings.TrimPrefix(line, p.indent())
			}
		}
		p.line(line)
	}
	p.lastLine = c.EndLine
	p.afterComment = true
}

// CollectComments returns every comment of src in source order, with the lines
// it occupied and whether it stood alone on a line. The AST holds no comments
// and this is what keeps them in a file the formatter rewrites.
func CollectComments(src string) []Comment {
	var out []Comment
	offset := 0
	for _, val := range parser.TokenGetAll(src) {
		a, ok := val.([]any)
		if !ok {
			offset += len(val.(string))
			continue
		}
		text := a[1].(string)
		if int(a[0].(int64)) == parser.T_COMMENT {
			line := int(a[2].(int64))
			trimmed := strings.TrimRight(text, "\r\n")
			lineStart := strings.LastIndex(src[:offset], "\n") + 1
			out = append(out, Comment{
				Text:    trimmed,
				Line:    line,
				EndLine: line + strings.Count(trimmed, "\n"),
				OwnLine: trimOpenTag(src[lineStart:offset]) == "",
			})
		}
		offset += len(text)
	}
	return out
}

// trimOpenTag returns the prefix before a comment, with an open tag removed.
func trimOpenTag(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	for _, tag := range []string{"<?php", "<?=", "<?"} {
		if strings.HasPrefix(trimmed, tag) {
			return strings.TrimSpace(trimmed[len(tag):])
		}
	}
	return trimmed
}
