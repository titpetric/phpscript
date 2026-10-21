package formatter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

type printer struct {
	buf       bytes.Buffer
	depth     int
	namespace string
	inPHP     bool
	spans     map[model.Stmt]model.SourceSpan
	// unsupported records the AST nodes the printer has no spelling for. The
	// formatter rewrites files in place, so printing a placeholder comment for
	// one would delete working code; Source reports them as an error instead
	// and leaves the file alone.
	unsupported []string
	// comments is the source comment stream, consumed in order: nextComment is
	// the first one not yet written out. lastLine is the source line of the
	// last thing written, so a blank line the author left
	// from one the printer would be inventing.
	// imports maps a fully-qualified name to the alias the file's use
	// statements gave it, so a reference prints in its own spelling.
	imports     map[string]string
	comments    []Comment
	nextComment int
	lastLine    int
	// afterComment records a comment as the last thing written, so a
	// declaration can follow its documentation without a blank line between.
	afterComment bool
}

// unsupportedNode records a node the printer cannot render and returns the
// placeholder text used in the (discarded) output.
func (p *printer) unsupportedNode(kind string, node any) string {
	text := fmt.Sprintf("/* unsupported %s %T */", kind, node)
	p.unsupported = append(p.unsupported, fmt.Sprintf("%s %T", kind, node))
	return text
}

func (p *printer) indent() string {
	return strings.Repeat("\t", p.depth)
}

func (p *printer) ensurePHP() {
	if p.inPHP {
		return
	}
	p.buf.WriteString("<?php\n")
	p.inPHP = true
}

func (p *printer) line(s string) {
	p.ensurePHP()
	p.buf.WriteString(p.indent())
	p.buf.WriteString(strings.TrimRight(s, " \t"))
	p.buf.WriteByte('\n')
}

func (p *printer) blank() {
	if p.inPHP {
		p.buf.WriteByte('\n')
	}
}

// stmts prints a statement list without leading or trailing blank lines. It
// retains one source blank line and adds separation after block statements and
// before a call that follows assignments.
func (p *printer) stmts(stmts []model.Stmt) {
	// A block opens with no blank line of its own, whatever the source did
	// between the brace and the first statement.
	p.lastLine = 0
	for i, s := range stmts {
		if i > 0 && p.blankBetween(stmts[i-1], s) {
			p.blank()
		}
		p.stmt(s)
	}
}

// body prints the statements of a block one level in, then writes out any
// comment left between the last statement and end, the line the block closes
// on. An end of zero leaves those comments for the enclosing block.
func (p *printer) body(stmts []model.Stmt, end int) {
	p.depth++
	p.stmts(stmts)
	if end > 0 {
		p.flushComments(end)
	}
	p.depth--
}

// stmtEnd is the line a statement closes on, read to place the comments
// written inside it.
func (p *printer) stmtEnd(s model.Stmt) int {
	return p.spans[s].End
}

// blankBetween reports the blank lines the printer adds of its own accord.
// stmt keeps the ones the author wrote, by comparing source lines.
func (p *printer) blankBetween(prev, next model.Stmt) bool {
	switch prev.(type) {
	case *model.ClassDecl, *model.InterfaceDecl:
		return false
	}
	if isBlockStmt(prev) {
		return true
	}
	if isDecl(next) || isFuncDecl(prev) {
		return true
	}
	_, assigned := prev.(*model.Assign)
	return assigned && isCallStmt(next)
}

func (p *printer) stmt(s model.Stmt) {
	if _, ok := s.(*model.InlineHTML); !ok {
		p.ensurePHP()
	}
	span, placed := p.spans[s]
	if placed {
		p.flushComments(span.Start)
		// A declaration follows its documentation without a gap; anything
		// else keeps the blank line the author left above it.
		if !p.afterComment || !isDecl(s) {
			p.blankFor(span.Start)
		}
	}
	p.printStmt(s)
	p.afterComment = false
	if placed {
		p.trailingComment(span.End)
		p.lastLine = span.End
	}
}
