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
	"strings"
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
	funcs map[funcKey]FuncSpan
	files map[string]bool
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

// FuncSpan is the source range of one declared function: a free function under
// its name, a method under Class::name. A per-function report charges the
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
// filename. Declarations are not executable and are skipped; their bodies'
// statements carry spans of their own. A statement already registered keeps its
// count, so re-including a cached program does not reset it.
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

// registerFuncs records the span of every declared function body: free
// functions from the statement list, methods from class declarations, named or
// anonymous. An abstract method declares no body and spans nothing to charge,
// so it is skipped. Interfaces declare only signatures and are skipped whole.
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
	// but carries a span like any parsed statement, so the sweep picks it up.
	// Methods were seen through their classes, which keeps them prefixed.
	for stmt := range program.SourceSpans {
		if fd, ok := stmt.(*model.FuncDecl); ok && !seen[fd] {
			add(fd, fd.Class)
		}
	}
}

// Files returns every registered filename, sorted. A file appears whether or
// not it holds an executable statement: a file of pure declarations registers
// no counts, and a report that renders only counted blocks would omit it,
// reading as a gap where there is nothing to run.
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

// Name is how a file is spelled in a profile: below the directory the command
// ran in, with no leading separator.
//
// The interpreter anchors an entrypoint at the source filesystem root for
// __FILE__ ("/app.php") and names an include as it resolved it ("app.php").
// Both are the same file, and a report that kept the two spellings apart would
// count it twice.
func Name(filename string) string {
	return strings.TrimPrefix(filename, "/")
}
