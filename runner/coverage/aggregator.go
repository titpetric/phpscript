package coverage

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

// keySep separates the fields of a composed map key. A filename may hold any
// byte a path may hold, colons and commas included, so the separator is one
// that cannot appear in one.
const keySep = "\x00"

// Aggregator accumulates the counts of many collectors for the lifetime of a
// process. Counts add: a statement two requests reached ran twice.
//
// A Collector keys statements by AST node, so one kept across requests grows
// with every re-parse. The aggregator keys them by the profile line they will
// be written as, which is bounded by the size of the application.
type Aggregator struct {
	mu     sync.Mutex
	blocks map[string]int
	files  map[string]bool
	funcs  map[string]int
}

// NewAggregator returns an empty aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{
		blocks: map[string]int{},
		files:  map[string]bool{},
		funcs:  map[string]int{},
	}
}

// Add folds one collector's counts in and returns. The collector is read, not
// retained, so the caller may drop it and the AST it keys on.
func (a *Aggregator) Add(c *Collector) {
	if c == nil {
		return
	}
	blocks := c.Blocks()
	files := c.Files()
	funcs := c.Functions()

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, b := range blocks {
		a.blocks[blockKey(b)] += b.Count
	}
	for _, f := range files {
		a.files[f] = true
	}
	for _, fn := range funcs {
		a.funcs[funcKeyOf(fn)] = fn.EndLine
	}
}

// Blocks returns the accumulated counts as profile entries, sorted by file and
// position.
func (a *Aggregator) Blocks() []Block {
	a.mu.Lock()
	defer a.mu.Unlock()
	blocks := make([]Block, 0, len(a.blocks))
	for key, count := range a.blocks {
		block, ok := parseBlockKey(key)
		if !ok {
			continue
		}
		block.Count = count
		blocks = append(blocks, block)
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

// Files returns every registered filename, sorted.
func (a *Aggregator) Files() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	files := make([]string, 0, len(a.files))
	for f := range a.files {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// Functions returns the declaration spans seen, sorted by file and position.
func (a *Aggregator) Functions() []FuncSpan {
	a.mu.Lock()
	defer a.mu.Unlock()
	funcs := make([]FuncSpan, 0, len(a.funcs))
	for key, end := range a.funcs {
		fn, ok := parseFuncKey(key)
		if !ok {
			continue
		}
		fn.EndLine = end
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

// Empty reports whether anything has been folded in yet. A server that served
// no PHP writes no profile rather than an empty one.
func (a *Aggregator) Empty() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.blocks) == 0 && len(a.files) == 0
}

// blockKey composes the identity of one statement range. The count is the map
// value and is not part of it.
func blockKey(b Block) string {
	return strings.Join([]string{
		b.File,
		strconv.Itoa(b.StartLine),
		strconv.Itoa(b.EndLine),
		strconv.Itoa(b.NumStmt),
	}, keySep)
}

func parseBlockKey(key string) (Block, bool) {
	parts := strings.Split(key, keySep)
	if len(parts) != 4 {
		return Block{}, false
	}
	start, err := strconv.Atoi(parts[1])
	if err != nil {
		return Block{}, false
	}
	end, err := strconv.Atoi(parts[2])
	if err != nil {
		return Block{}, false
	}
	numStmt, err := strconv.Atoi(parts[3])
	if err != nil {
		return Block{}, false
	}
	return Block{File: parts[0], StartLine: start, EndLine: end, NumStmt: numStmt}, true
}

// funcKeyOf composes the identity of one declaration. The end line is the map
// value, because a declaration that grew between parses keeps its start.
func funcKeyOf(fn FuncSpan) string {
	return strings.Join([]string{fn.File, strconv.Itoa(fn.StartLine), fn.Name}, keySep)
}

func parseFuncKey(key string) (FuncSpan, bool) {
	parts := strings.SplitN(key, keySep, 3)
	if len(parts) != 3 {
		return FuncSpan{}, false
	}
	start, err := strconv.Atoi(parts[1])
	if err != nil {
		return FuncSpan{}, false
	}
	return FuncSpan{File: parts[0], Name: parts[2], StartLine: start}, true
}
