// Package coverage is the statement-coverage extension of the runner: a
// collector the interpreter reports statement executions to, and the profile
// blocks a host renders from it.
//
// Coverage is an interpreter feature. flatstack is a performance-oriented
// backend and carries no coverage support; a runner holding a collector
// executes interpreted (see runner.Runtime.SetCoverage).
package coverage

import (
	"sort"
	"sync"

	"github.com/titpetric/phpscript/model"
)

// Collector accumulates statement execution counts, keyed by the statement
// nodes the parser produced.
//
// A program is registered before it runs, seeding every executable statement at
// count zero under the filename it was loaded from. The entrypoint and every
// include register, so the files in a profile are the files get_included_files
// reports, and an included-but-unexecuted statement still appears, at zero.
type Collector struct {
	mu    sync.Mutex
	stmts map[model.Stmt]*coverStmt
}

// coverStmt is one registered statement: the file it was parsed from, the line
// range charged for it, and how many times the interpreter reached it.
type coverStmt struct {
	file  string
	span  model.SourceSpan
	count int
}

// Block is one line-range entry of a coverage profile. Lines only: the parser
// records no columns, so a profile writer chooses them (the test command
// derives them from the source text).
type Block struct {
	File      string
	StartLine int
	EndLine   int
	NumStmt   int
	Count     int
}

// New returns an empty collector.
func New() *Collector {
	return &Collector{stmts: map[model.Stmt]*coverStmt{}}
}

// Register seeds every executable statement of program at count zero under
// filename. Declarations are not executable and are skipped; their bodies'
// statements carry spans of their own. A statement already registered keeps its
// count, so re-including a cached program does not reset it.
func (c *Collector) Register(filename string, program *model.Program) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for stmt, span := range program.SourceSpans {
		if !coverable(stmt) {
			continue
		}
		if _, ok := c.stmts[stmt]; ok {
			continue
		}
		c.stmts[stmt] = &coverStmt{file: filename, span: coverSpan(stmt, span)}
	}
}

// Hit charges one execution to a registered statement. An unregistered
// statement (a declaration, or a program built without the parser) is not
// counted.
func (c *Collector) Hit(s model.Stmt) {
	c.mu.Lock()
	if entry, ok := c.stmts[s]; ok {
		entry.count++
	}
	c.mu.Unlock()
}

// Blocks renders the collected counts as profile entries, sorted by file and
// position. Statements sharing a line range merge into one block: NumStmt adds
// up, and Count is the largest of theirs, which is how many times the line was
// reached — summing would charge `$a = 1; $b = 2;` twice per pass.
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

// coverable reports whether a statement executes. A declaration does not: the
// interpreter hoists it and exec passes it by, so counting it would mark a
// never-called function's signature covered while its body stays at zero.
func coverable(s model.Stmt) bool {
	switch s.(type) {
	case *model.FuncDecl, *model.ClassDecl, *model.InterfaceDecl, *model.Use:
		return false
	}
	return true
}

// coverSpan is the line range charged for a statement. A compound statement's
// parsed span covers its whole body, but the body's statements carry spans of
// their own, so only the header line is charged here: an untaken branch must
// not read as covered because its `if` was reached.
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
