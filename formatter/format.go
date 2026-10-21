// Package formatter pretty-prints phpscript ASTs in a gofmt-like style:
// hard tabs, canonical `function` keywords, parentheses on control structures,
// semicolons, collapsed blank lines, and a trailing newline. Class, function,
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
	// skips. Source puts it back on the formatted output, so it costs the
	// script nothing of its formatting.
	if shebang, rest, ok := splitShebang(src); ok {
		out, err := Source(rest)
		if err != nil {
			return "", err
		}
		return shebang + "\n" + out, nil
	}
	// Files that start outside PHP are templates. Formatting their PHP blocks
	// independently is not supported yet, so leave the entire file unchanged.
	if !strings.HasPrefix(src, "<?php") {
		return src, nil
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	out, unsupported := printProgram(prog, Options{Comments: CollectComments(src)})
	// Formatting rewrites the file in place, so a node the printer cannot
	// spell has to stop the rewrite: emitting the placeholder would delete
	// working code.
	if len(unsupported) > 0 {
		return "", fmt.Errorf("cannot format: unsupported %s", strings.Join(unsupported, ", "))
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	return strings.ReplaceAll(out, "\r", "\n"), nil
}

// splitShebang separates a leading `#!` interpreter line from the rest of the
// file. It reports false for a file that does not have one.
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

// Print renders prog as formatted PHP source, a node it cannot spell as a
// placeholder comment. Source fails on such a file instead.
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
	p := &printer{depth: 0, namespace: prog.Namespace, inPHP: startsInPHP, spans: prog.SourceSpans, comments: opts.Comments}
	p.imports = collectImports(stmts)
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
	// Includes ahead of imports, as their own paragraphs. Printed here rather
	// than reordered in place because the blank line between them is the
	// point: two groups a reader can take in at a glance beats one block
	// whose order happens to be right.
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

// splitDeclarePreamble separates the leading `declare(...);` statements from
// the rest of the program. PHP requires them to be the first statement in the
// file, ahead of the namespace declaration.
func splitDeclarePreamble(stmts []model.Stmt) (preamble, rest []model.Stmt) {
	for i, s := range stmts {
		d, ok := s.(*model.Declare)
		if !ok || d.Block {
			return stmts[:i], stmts[i:]
		}
	}
	return stmts, nil
}

// collectImports maps every name a file imports to the alias it gave it, so
// the printer can write a reference the way the author did and not the way
// the parser resolved it.
//
// The alias of an unaliased import is its last segment, the name the
// short name in the body already means.
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

// splitImportPreamble separates a file's leading includes from its leading
// imports, and answers what follows both.
//
// A `use` is a compile-time alias: it binds a short name to a long one and
// loads nothing, so it can sit anywhere and is legal before the include that
// will eventually define what it names. That is exactly why the order is
// worth fixing here, and not left to whoever types it: a reader
// scanning the top of a file learns what the file pulls in before what it
// renames, and one order for every file is one less thing to decide.
//
// This reorders the leading run alone, and only across the include/use
// boundary. Includes keep their order among themselves, because an include
// runs code and the next one may depend on it; imports keep theirs. A `use`
// further down the file, after real work has started, stays where the author
// put it.
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
			// include "x";` - is an expression, and the parser wraps it. It
			// remains the file pulling something in.
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
