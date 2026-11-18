// Package coverage is the statement-coverage extension of the runner: a
// collector the interpreter reports statement executions to, and the profile
// blocks a host renders from it.
//
// Coverage is an interpreter feature. flatstack is a performance-oriented
// backend and supports no coverage; a runner holding a collector
// executes interpreted (see runner.Runtime.SetCoverage).
package coverage

import (
	"sort"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/model"
)

// Collector accumulates statement execution counts, keyed by the statement
// nodes the parser produced.
//
// Register runs before the program, seeding each executable statement at count
// zero by its filename. The entrypoint, every include and the files in a
// profile match get_included_files. An included-but-unexecuted statement still
// appears, at zero.
type Collector struct {
	mu    sync.Mutex
	stmts map[model.Stmt]*coverStmt
	funcs map[funcKey]FuncSpan
	files map[string]bool
}

// coverStmt holds one registered statement's file, line range and execution count.
type coverStmt struct {
	file  string
	span  model.SourceSpan
	count int
}

// Block holds a line-range entry of a coverage profile. Lines only: the parser
// records no columns; the profile writer chooses them. The test command derives
// them from the source text.
type Block struct {
	File      string
	StartLine int
	EndLine   int
	NumStmt   int
	Count     int
}

// FuncSpan is the source range of one declared function: a free function by
// the bare name, a method under Class::name. A per-function report charges the
// blocks inside the range to it.
type FuncSpan struct {
	File      string
	Name      string
	StartLine int
	EndLine   int
}

// funcKey identifies a declaration across re-registrations of the same file.
type funcKey struct {
	file  string
	name  string
	start int
}

// New returns an empty collector.
func New() *Collector {
	return &Collector{
		stmts: map[model.Stmt]*coverStmt{},
		funcs: map[funcKey]FuncSpan{},
		files: map[string]bool{},
	}
}

// Register seeds every executable statement of program at count zero under
// filename. A declaration does not execute, and Register skips it. The body's
// statements register under separate spans. A statement already registered keeps
// its count and re-including a cached program does not reset it.
func (c *Collector) Register(filename string, program *model.Program) {
	filename = Name(filename)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[filename] = true
	for stmt, span := range program.SourceSpans {
		if !coverable(stmt) {
			continue
		}
		if _, ok := c.stmts[stmt]; ok {
			continue
		}
		c.stmts[stmt] = &coverStmt{file: filename, span: coverSpan(stmt, span)}
	}
	c.registerFuncs(filename, program)
}

// registerFuncs records the body span of every declared function and method.
func (c *Collector) registerFuncs(filename string, program *model.Program) {
	seen := map[*model.FuncDecl]bool{}
	add := func(fd *model.FuncDecl, class string) {
		seen[fd] = true
		if fd.Abstract || len(fd.Body) == 0 {
			return
		}
		span, ok := program.SourceSpans[fd]
		if !ok {
			return
		}
		name := fd.Name
		if class != "" {
			name = class + "::" + fd.Name
		}
		key := funcKey{file: filename, name: name, start: span.Start}
		if _, done := c.funcs[key]; done {
			return
		}
		c.funcs[key] = FuncSpan{File: filename, Name: name, StartLine: span.Start, EndLine: span.End}
	}
	classes := make([]*model.ClassDecl, 0, len(program.AnonClasses))
	classes = append(classes, program.AnonClasses...)
	for _, stmt := range program.Stmts {
		switch n := stmt.(type) {
		case *model.FuncDecl:
			add(n, n.Class)
		case *model.ClassDecl:
			classes = append(classes, n)
		}
	}
	for _, class := range classes {
		for _, method := range class.Methods {
			add(method, class.Name)
		}
	}
	// A function declared inside another statement's body is in no list above
	// but holds a span like any parsed statement and the sweep picks it up.
	// A method arrives through its class, which keeps it prefixed.
	for stmt := range program.SourceSpans {
		if fd, ok := stmt.(*model.FuncDecl); ok && !seen[fd] {
			add(fd, fd.Class)
		}
	}
}

// Files returns every registered filename, sorted. A file appears whether it
// holds an executable statement or not.
func (c *Collector) Files() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	files := make([]string, 0, len(c.files))
	for f := range c.files {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// Functions returns the registered declaration spans, sorted by file and
// position. Every function of every registered file appears, called or not:
// an uncalled function is the zero row a per-function report exists to show.
func (c *Collector) Functions() []FuncSpan {
	c.mu.Lock()
	defer c.mu.Unlock()
	funcs := make([]FuncSpan, 0, len(c.funcs))
	for _, fn := range c.funcs {
		funcs = append(funcs, fn)
	}
	sort.Slice(funcs, func(i, j int) bool {
		if funcs[i].File != funcs[j].File {
			return funcs[i].File < funcs[j].File
		}
		if funcs[i].StartLine != funcs[j].StartLine {
			return funcs[i].StartLine < funcs[j].StartLine
		}
		return funcs[i].Name < funcs[j].Name
	})
	return funcs
}

// Hit charges one execution to a registered statement. An unregistered
// statement (a declaration, a program built without the parser) is not counted.
func (c *Collector) Hit(s model.Stmt) {
	c.mu.Lock()
	if entry, ok := c.stmts[s]; ok {
		entry.count++
	}
	c.mu.Unlock()
}

// Blocks renders the collected counts as profile entries, sorted by file and
// position. Statements sharing a line range merge into a single block: NumStmt
// adds up, and Count is the largest of theirs, the number of times the line runs;
// summing would charge `$a = 1; $b = 2;` twice per pass.
func (c *Collector) Blocks() []Block {
	c.mu.Lock()
	defer c.mu.Unlock()

	type key struct {
		file       string
		start, end int
	}
	merged := map[key]*Block{}
	for _, entry := range c.stmts {
		k := key{file: entry.file, start: entry.span.Start, end: entry.span.End}
		block, ok := merged[k]
		if !ok {
			block = &Block{File: entry.file, StartLine: entry.span.Start, EndLine: entry.span.End}
			merged[k] = block
		}
		block.NumStmt++
		if entry.count > block.Count {
			block.Count = entry.count
		}
	}

	blocks := make([]Block, 0, len(merged))
	for _, block := range merged {
		blocks = append(blocks, *block)
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].File != blocks[j].File {
			return blocks[i].File < blocks[j].File
		}
		if blocks[i].StartLine != blocks[j].StartLine {
			return blocks[i].StartLine < blocks[j].StartLine
		}
		return blocks[i].EndLine < blocks[j].EndLine
	})
	return blocks
}

// coverable reports whether a statement executes; a declaration does not.
func coverable(s model.Stmt) bool {
	switch s.(type) {
	case *model.FuncDecl, *model.ClassDecl, *model.InterfaceDecl, *model.Use:
		return false
	}
	return true
}

// coverSpan returns the header line alone for a compound statement.
func coverSpan(s model.Stmt, span model.SourceSpan) model.SourceSpan {
	switch n := s.(type) {
	case *model.If, *model.For, *model.Foreach, *model.DoWhile, *model.Switch, *model.Try:
		return model.SourceSpan{Start: span.Start, End: span.Start}
	case *model.Declare:
		if n.Block {
			return model.SourceSpan{Start: span.Start, End: span.Start}
		}
	}
	return span
}

// Name spells a file for a profile, without a leading separator.
func Name(filename string) string {
	return strings.TrimPrefix(filename, "/")
}
