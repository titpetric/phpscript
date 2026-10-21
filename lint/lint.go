package lint

import (
	"fmt"
	"os"
	"strings"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/tests"
)

// Diagnostic is one lint finding.
type Diagnostic struct {
	File    string
	Line    int
	Message string

	// Fatal marks a finding the program cannot survive, as against one it
	// might: a call to a function some host registers is a warning because the
	// linter cannot see every binding, but the runtime refuses a redeclaration
	// whatever a host registers. A fatal finding fails the lint run.
	Fatal bool
}

// String renders the finding as a line a terminal prints: file, line, message,
// with the position left out when the finding is about no file in particular.
func (d Diagnostic) String() string {
	if d.File != "" {
		return fmt.Sprintf("%s:%d: %s", d.File, d.Line, d.Message)
	}
	return fmt.Sprintf("line %d: %s", d.Line, d.Message)
}

// File parses and lints one PHP source file.
func File(name, src string) ([]Diagnostic, error) {
	src, err := lintSource(name, src)
	if err != nil {
		return []Diagnostic{{File: name, Line: 1, Message: fmt.Sprintf("fixture parse error: %v", err)}}, nil
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return []Diagnostic{{File: name, Line: 1, Message: fmt.Sprintf("parse error: %v", err)}}, nil
	}
	var out []Diagnostic
	lintJSONFlags(name, src, &out)
	lintRoutes(name, src, &out)
	lintInterfaces(name, prog, &out)
	lintStmts(name, prog, &out)
	lintReferences(name, prog, &out)
	lintUndefinedNames(name, prog, &out)
	lintRedeclared(name, prog, &out)
	lintTypeReassign(name, prog, &out)
	return out, nil
}

// lintJSONFlags reports a JSON_* constant.
//
// Nothing here defines a JSON_* constant, so the name evaluates to null and
// json_encode ignores the argument. The call runs and encodes correctly, so
// this is a warning: nothing fails, and the author named a formatting the
// encoder does not vary. See docs/design.md.
//
// The scan is over tokens and not the AST: a constant can appear in any
// expression, and T_STRING already tells a bare name apart from the same text
// inside a string literal, so defined("JSON_PRETTY_PRINT") is not a use.
func lintJSONFlags(file, src string, out *[]Diagnostic) {
	for _, token := range parser.TokenGetAll(src) {
		triple, ok := token.([]any)
		if !ok || len(triple) != 3 {
			continue
		}
		// A token id and line are int64, the type a PHP integer has here.
		id, _ := triple[0].(int64)
		text, _ := triple[1].(string)
		line, _ := triple[2].(int64)
		if id != int64(parser.T_STRING) || !strings.HasPrefix(text, "JSON_") {
			continue
		}
		*out = append(*out, Diagnostic{
			File: file,
			Line: int(line),
			Message: fmt.Sprintf("%s is not defined and the argument is ignored: the JSON encoding is not "+
				"configurable. Drop it, and indent downstream if the output has to be read; see docs/design.md.", text),
		})
	}
}

// lintRoutes reports an @route path whose parameters the router cannot
// answer for.
//
// The two routers this runtime registers on disagree about what a {...}
// segment may say, and neither refuses the spellings the other does not
// understand in a way an author sees: chi takes {module=users} as a parameter
// of that literal name, matches every request to the segment and exports
// nothing. Registration skips such a route; this is the diagnostic that names
// the file and the line before it gets that far.
func lintRoutes(file, src string, out *[]Diagnostic) {
	for _, route := range annotations.ParseRoutes([]byte(src)) {
		if _, err := model.ParseRoutePath(route.Path); err != nil {
			*out = append(*out, Diagnostic{File: file, Line: route.Line, Message: err.Error()})
		}
	}
}

// lintInterfaces reports a class that declares `implements` and does not
// declare a method the interface names. The runtime raises a RuntimeException
// for the same condition, so the finding is the warning that arrives before
// the program runs.
func lintInterfaces(file string, prog *model.Program, out *[]Diagnostic) {
	for _, v := range model.CheckInterfaces(prog.Stmts) {
		*out = append(*out, Diagnostic{
			File:    file,
			Line:    prog.SourceSpans[v.Decl].Start,
			Message: v.String(),
		})
	}
}

// FlatstackFile checks whether a single PHP source file is compatible with flatstack engine.
func FlatstackFile(name, src string) (Diagnostic, error) {
	src, err := lintSource(name, src)
	if err != nil {
		return Diagnostic{File: name, Line: 1, Message: fmt.Sprintf("fixture parse error: %v", err)}, nil
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return Diagnostic{File: name, Line: 1, Message: fmt.Sprintf("parse error: %v", err)}, nil
	}
	if err := flatstack.Supports(prog); err != nil {
		return Diagnostic{File: name, Line: 1, Message: fmt.Sprintf("[flatstack unsupported] %v", err)}, nil
	}
	return Diagnostic{File: name, Line: 1, Message: "[flatstack compatible] 100% compatible with flatstack bytecode engine"}, nil
}

// FlatstackPaths checks flatstack compatibility for all files matched by paths.
func FlatstackPaths(paths []string) ([]Diagnostic, error) {
	files, err := list.ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	var out []Diagnostic
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		diag, err := FlatstackFile(file, string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, diag)
	}
	return out, nil
}

// Paths lints each file the Go-style path patterns select.
func Paths(paths []string) ([]Diagnostic, error) {
	files, err := list.ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	var out []Diagnostic
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		diags, err := File(file, string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, diags...)
	}
	return out, nil
}

func lintSource(name, src string) (string, error) {
	if !strings.HasSuffix(name, ".phpt") {
		return src, nil
	}

	fixture, err := tests.ParseFixture([]byte(src), name)
	if err != nil {
		return "", err
	}

	// Keep the PHP section at its physical position in the fixture so lint
	// diagnostics point to lines in the .phpt file and not section-relative
	// lines.
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	phpStart := strings.Index(normalized, "\n---\n") + len("\n---\n")
	lineOffset := strings.Count(normalized[:phpStart], "\n")
	if strings.HasPrefix(normalized[phpStart:], "\n") {
		lineOffset++ // ParseFixture trims one optional leading section newline.
	}
	return strings.Repeat("\n", lineOffset) + fixture.PHP, nil
}
