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
// anything. Every .php file below Root is parsed into Includes and compiled
// into Exprs, so the parse and the compile a request pays for today are paid
// once at startup instead. Nothing is invalidated afterwards: an edited file
// is picked up by a reload or a restart.
type Precompiler struct {
	// Root is the application root the tree is walked from.
	Root fs.FS

	// Includes receives every file that parsed, keyed by its path below Root.
	// That is the key Runtime.LoadFile and include/require both resolve to,
	// because a path is cleaned against the root before it is read.
	Includes *IncludeCache

	// Exprs receives the flat bytecode of each parsed program. Nil parses
	// without compiling: a host running the interpreter never reads bytecode,
	// and holding it is resident memory for nothing.
	Exprs *ExprCache

	// Workers bounds how many files are parsed at once. Zero is GOMAXPROCS,
	// which is the parallelism the machine has for work that is all CPU.
	Workers int
}

// Run parses and compiles the tree, and reports how many files it cached.
func (p Precompiler) Run() int {
	if p.Root == nil || p.Includes == nil {
		return 0
	}

	files := p.sources()
	if len(files) == 0 {
		return 0
	}

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

// sources lists the .php files below Root. A directory that cannot be read is
// skipped rather than ending the walk, so one unreadable folder costs its own
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
// A file that does not parse, and a program the flat compiler rejects, are
// left out. Both are the state the tree is in without precompilation, and a
// request that reaches one gets the same answer it gets today: startup is not
// where a source error in a file nothing may ever request is reported.
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
