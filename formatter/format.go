// Package formatter pretty-prints phpscript ASTs in a gofmt-like style:
// hard tabs, canonical `function` keywords, parentheses on control structures,
// semicolons, collapsed blank lines, plus a trailing newline. Class, function,
// and control-structure opening braces stay on the declaration line.
package formatter

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

// Source parses src and pretty-prints the AST.
func Source(src string) (string, error) {
	// An executable script starts with an interpreter line, which the lexer
	// skips. Source puts it back on the formatted output and it costs the
	// script nothing of its formatting.
	if shebang, rest, ok := splitShebang(src); ok {
		out, err := Source(rest)
		if err != nil {
			return "", err
		}
		return shebang + "\n" + out, nil
	}
	// Files that start outside PHP are templates. Formatting their PHP blocks
	// independently is not supported yet and leave the entire file unchanged.
	if !strings.HasPrefix(src, "<?php") {
		return src, nil
	}
	// A line that opens and closes PHP is template markup: the tag pair, the
	// code between and the text around them are the page it renders. The
	// printer rewrites statements. A template's lines keep their contents, and
	// only the leading indentation moves. See template.go.
	if hasInlinePHPBlock(src) {
		return alignTemplate(src, isGoRuntimeSource(src)), nil
	}
	prog, goRuntime, err := parseSource(src)
	if err != nil {
		return "", err
	}
	out, unsupported := printProgram(prog, Options{Comments: CollectComments(src), GoRuntime: goRuntime})
	// Formatting rewrites the file in place and a node the printer cannot
	// spell has to stop the rewrite: emitting the placeholder would delete
	// working code.
	if len(unsupported) > 0 {
		return "", fmt.Errorf("cannot format: unsupported %s", strings.Join(unsupported, ", "))
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	return strings.ReplaceAll(out, "\r", "\n"), nil
}

// hasInlinePHPBlock reports whether a line holds both tags of a PHP block.
func hasInlinePHPBlock(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		open := strings.Contains(line, "<?php") || strings.Contains(line, "<?=")
		if open && strings.Contains(line, "?>") {
			return true
		}
	}
	return false
}

// parseSource reads src as php, or as go when php refuses it.
func parseSource(src string) (*model.Program, bool, error) {
	prog, err := parser.Parse(src)
	if err == nil {
		return prog, usesGoRuntime(prog), nil
	}
	// A go runtime file spells what php cannot: `import "net/http";`, a
	// `{key: value}` map and a `[]T{...}` literal.
	if goProg, goErr := parser.ParseFileOptions("input.php", src, parser.Options{GoRuntime: true}); goErr == nil {
		return goProg, true, nil
	}
	return nil, false, err
}

// usesGoRuntime reports whether prog imports the go runtime namespace.
func usesGoRuntime(prog *model.Program) bool {
	for _, s := range prog.Stmts {
		use, ok := s.(*model.Use)
		if !ok {
			continue
		}
		for _, imp := range use.Imports {
			if strings.EqualFold(strings.TrimPrefix(imp.Path, `\`), `Runtime\Go`) {
				return true
			}
		}
	}
	return false
}

// splitShebang separates a leading interpreter line, reporting false when absent.
func splitShebang(src string) (shebang, rest string, ok bool) {
	if !strings.HasPrefix(src, "#!") {
		return "", src, false
	}
	line, rest, found := strings.Cut(src, "\n")
	if !found {
		return "", src, false
	}
	return strings.TrimRight(line, "\r"), rest, true
}

// Print returns prog as formatted PHP source, an unspellable node as a comment.
func Print(prog *model.Program, opts Options) string {
	out, _ := printProgram(prog, opts)
	return out
}

// printProgram renders prog and reports the nodes it could not render.
func printProgram(prog *model.Program, opts Options) (string, []string) {
	startsInPHP := prog.Namespace != "" || len(opts.Comments) > 0 || len(prog.Stmts) == 0
	if len(prog.Stmts) > 0 {
		_, startsWithHTML := prog.Stmts[0].(*model.InlineHTML)
		startsInPHP = startsInPHP || !startsWithHTML
	}
	stmts := prog.Stmts
	if len(stmts) > 0 {
		if html, ok := stmts[len(stmts)-1].(*model.InlineHTML); ok && strings.TrimSpace(html.Text) == "" {
			stmts = stmts[:len(stmts)-1]
		}
	}
	p := &printer{depth: 0, namespace: prog.Namespace, inPHP: startsInPHP, spans: prog.SourceSpans, comments: opts.Comments, goRuntime: opts.GoRuntime}
	p.imports = collectImports(stmts)
	p.goImports = collectGoImports(stmts)
	if startsInPHP {
		p.line("<?php")
		p.blank()
	}
	// `declare` has to come before the namespace declaration, which the
	// printer takes from Program.Namespace and not from the statement list.
	directives, stmts := splitDeclarePreamble(stmts)
	if len(directives) > 0 {
		p.stmts(directives)
		p.blank()
	}
	if prog.Namespace != "" {
		p.flushComments(prog.NamespaceLine)
		p.blankFor(prog.NamespaceLine)
		p.line("namespace " + prog.Namespace + ";")
		p.lastLine = prog.NamespaceLine
		p.blank()
	}
	// Includes ahead of imports, as their own paragraphs. The printer groups
	// them here and leaves the statement list alone. Two groups split by a
	// blank line read at a glance where one ordered block does not.
	if includes, uses, rest := splitImportPreamble(stmts); len(includes) > 0 && len(uses) > 0 {
		p.stmts(includes)
		p.blank()
		p.stmts(uses)
		stmts = rest
		if len(stmts) > 0 {
			p.blank()
		}
	}
	p.stmts(stmts)
	// Anything written below the last statement, a trailing note or a
	// commented-out block, still belongs to the file.
	p.flushComments(0)
	out := collapseBlankLines(p.buf.String())
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, p.unsupported
}

// splitDeclarePreamble separates the leading declare statements PHP requires first.
func splitDeclarePreamble(stmts []model.Stmt) (preamble, rest []model.Stmt) {
	for i, s := range stmts {
		d, ok := s.(*model.Declare)
		if !ok || d.Block {
			return stmts[:i], stmts[i:]
		}
	}
	return stmts, nil
}

// collectImports maps every imported name to its alias, the last segment when unaliased.
func collectImports(stmts []model.Stmt) map[string]string {
	out := map[string]string{}
	for _, s := range stmts {
		use, ok := s.(*model.Use)
		if !ok {
			continue
		}
		for _, imp := range use.Imports {
			alias := imp.Alias
			if alias == "" {
				alias = shortName(imp.Path)
			}
			out[strings.ToLower(strings.TrimPrefix(imp.Path, `\`))] = alias
		}
	}

	return out
}

// splitImportPreamble separates the leading includes from imports, preserving each group's order.
func splitImportPreamble(stmts []model.Stmt) (includes, uses, rest []model.Stmt) {
	from := len(stmts)
	for i, s := range stmts {
		switch node := s.(type) {
		case *model.Use:
			uses = append(uses, s)
			continue
		case *model.Include:
			includes = append(includes, s)
			continue
		case *model.ExprStmt:
			// An include written where the code reads a value - `$ok =
			// include "x";` - is an expression the parser wraps in an
			// ExprStmt. It remains the file pulling something in.
			if _, ok := node.X.(*model.Include); ok {
				includes = append(includes, s)
				continue
			}
		}
		from = i
		break
	}

	return includes, uses, stmts[from:]
}

func collapseBlankLines(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	var out []string
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
