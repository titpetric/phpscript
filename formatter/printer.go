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
	// formatter rewrites files in place and printing a placeholder comment for
	// one would delete working code; Source reports them as an error instead
	// and leaves the file alone.
	unsupported []string
	// comments is the source comment stream, consumed in order: nextComment is
	// the first one not yet written out. lastLine is the source line of the
	// last thing written and a blank line the author left
	// from one the printer would be inventing.
	// imports maps a fully-qualified name to the alias the file's use
	// statements gave it and a reference prints in its own spelling.
	imports map[string]string
	// goImports maps the area of an `import "net/http"` statement to the name
	// it bound, for the members that print as the source wrote them. Nil for a
	// file that writes none. See goimport.go.
	goImports   map[string]string
	comments    []Comment
	nextComment int
	lastLine    int
	// afterComment records a comment as the last thing written and a
	// declaration can follow its documentation without a blank line between.
	afterComment bool
	// collapseBlank drops the source blank line above a call written directly
	// under an assignment, which reads as one setup and run unit.
	collapseBlank bool
	// goRuntime spells statements without semicolons and members in lowerCamelCase.
	goRuntime bool
}

// term ends a statement; the go runtime takes go's implicit semicolon.
func (p *printer) term() string {
	if p.goRuntime {
		return ""
	}
	return ";"
}

// member spells a member name in the runtime's style.
func (p *printer) member(name string) string {
	if p.goRuntime {
		return lowerMember(name)
	}
	return name
}

// unsupportedNode records a node the printer cannot render and returns its placeholder text.
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

// stmts prints a statement list without leading or trailing blank lines, keeping one source blank.
func (p *printer) stmts(stmts []model.Stmt) {
	// A block opens with no leading blank line, whatever the source did
	// between the brace and the first statement.
	p.lastLine = 0
	for i, s := range stmts {
		if i > 0 && p.blankBetween(stmts[i-1], s) {
			p.blank()
		}
		p.collapseBlank = i > 0 && isAssignCall(stmts[i-1], s)
		p.stmt(s)
	}
}

// body prints a block's statements one level in, then the comments left before its closing line.
func (p *printer) body(stmts []model.Stmt, end int) {
	p.depth++
	p.stmts(stmts)
	if end > 0 {
		p.flushComments(end)
	}
	p.depth--
}

// stmtEnd returns the line a statement closes on, which places its enclosed comments.
func (p *printer) stmtEnd(s model.Stmt) int {
	return p.spans[s].End
}

// blankBetween reports whether the printer adds a blank line between two statements.
func (p *printer) blankBetween(prev, next model.Stmt) bool {
	switch prev.(type) {
	case *model.ClassDecl, *model.InterfaceDecl:
		return false
	}
	if isBlockStmt(prev) {
		return true
	}
	return isDecl(next) || isFuncDecl(prev)
}

// isAssignCall reports a call written directly under an assignment.
func isAssignCall(prev, next model.Stmt) bool {
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
		if (!p.afterComment || !isDecl(s)) && !p.collapseBlank {
			p.blankFor(span.Start)
		}
	}
	p.collapseBlank = false
	p.printStmt(s)
	p.afterComment = false
	if placed {
		p.trailingComment(span.End)
		p.lastLine = span.End
	}
}
