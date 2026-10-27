package runner

import (
	"io/fs"
	"path"
	goruntime "runtime"
	"sync"
	"sync/atomic"

	"github.com/titpetric/phpscript/parser"
)

// Precompiler fills the caches a source tree serves from, before it serves
// anything. The pass parses every .php file below Root into Includes and
// compiles it into Exprs and startup pays once for the parse and the compile a
// request pays for today. No call invalidates an entry afterwards: a reload or
// a restart takes up an edited file.
type Precompiler struct {
	// Root is the application root the walk starts at.
	Root fs.FS

	// Includes receives every file that parsed, keyed by its path below Root.
	// That is the key Runtime.LoadFile and include/require both resolve to,
	// because the runtime cleans a path against the root before the read.
	Includes *IncludeCache

	// Exprs receives the flat bytecode of each parsed program. Nil parses
	// without compiling: a host running the interpreter never reads bytecode,
	// and holding bytecode is resident memory for nothing.
	Exprs *ExprCache

	// Workers bounds how many files the pass parses at once. Zero is GOMAXPROCS,
	// which is the parallelism the machine has for work that is all CPU.
	Workers int
}

// Run parses and compiles the tree, and reports how many files it cached. The
// pass records the heap it added on the include cache, where
// opcache_get_status() reads it.
func (p Precompiler) Run() int {
	if p.Root == nil || p.Includes == nil {
		return 0
	}

	files := p.sources()
	if len(files) == 0 {
		return 0
	}

	// This measures the cost and estimates nothing. A parsed program is a
	// graph of nodes with no size of its own to add up, and the number worth
	// reporting is what the process holds because of the pass. Both reads
	// follow a collection and the figure is live heap on each side and never
	// whatever the collector has yet to sweep.
	before := liveHeap()
	defer func() { p.Includes.Account(liveHeap() - before) }()

	workers := p.Workers
	if workers <= 0 {
		workers = goruntime.GOMAXPROCS(0)
	}
	workers = min(workers, len(files))

	var cached atomic.Int64
	names := make(chan string)
	go func() {
		defer close(names)
		for _, name := range files {
			names <- name
		}
	}()

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for name := range names {
				if p.compile(name) {
					cached.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	return int(cached.Load())
}

// liveHeap answers the bytes reachable after a collection.
func liveHeap() int64 {
	goruntime.GC()
	var stats goruntime.MemStats
	goruntime.ReadMemStats(&stats)
	return int64(stats.HeapAlloc)
}

// sources lists the .php files below Root. The walk passes over a directory it
// cannot read and runs to the end and one unreadable folder costs its own
// files and not the tree.
func (p Precompiler) sources() []string {
	var files []string
	_ = fs.WalkDir(p.Root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || path.Ext(name) != ".php" {
			return nil
		}
		files = append(files, name)
		return nil
	})
	return files
}

// compile parses one file into the caches and reports whether it cached it.
//
// The pass leaves out a file that does not parse, and a program the flat
// compiler rejects. Both are the state the tree is in without precompilation,
// and a request that reaches one gets the same answer it gets today: startup
// stays quiet about a source error in a file nothing may ever request.
func (p Precompiler) compile(name string) bool {
	src, err := fs.ReadFile(p.Root, name)
	if err != nil {
		return false
	}
	program, err := parser.ParseFile(rootPath(name), string(src))
	if err != nil {
		return false
	}
	p.Includes.Set(name, program)
	if p.Exprs != nil {
		_ = p.Exprs.EnsureFlat(program)
	}
	return true
}
