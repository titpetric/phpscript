package server

import (
	"log"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
)

// sharedCaches puts a site's annotated files on its file handler's caches.
func sharedCaches(files *handler, options []annotations.Option) []annotations.Option {
	return append(options[:len(options):len(options)],
		annotations.WithIncludeCache(files.includeCache),
		annotations.WithExprCache(files.exprCache),
	)
}

// precompile fills a site's caches, never failing startup on a broken file.
func precompile(files *handler, name string) {
	if !files.runnerOptions.Precompile {
		return
	}

	pass := runner.Precompiler{Root: files.root, Includes: files.includeCache, GoRuntime: files.runnerOptions.GoRuntime()}
	if files.flatstack {
		// Bytecode for a site running the interpreter is resident memory that
		// no request executes; only a site that reads it pays the flat compile.
		pass.Exprs = files.exprCache
	}
	cached := pass.Run()
	log.Printf("precompiled %s: %d files, %d bytes", name, cached, files.includeCache.Precompiled())
}
