package server

import (
	"log"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
)

// sharedCaches puts a site's annotated files on the caches its file handler
// serves from. The routed endpoints and the document root read one source
// tree and one precompile pass covers both and neither parses it again.
func sharedCaches(files *handler, options []annotations.Option) []annotations.Option {
	return append(options[:len(options):len(options)],
		annotations.WithIncludeCache(files.includeCache),
		annotations.WithExprCache(files.exprCache),
	)
}

// precompile parses a site's tree into its caches before the site serves a
// request, when runner.precompile named it. A site that did not stays
// lazy: it parses a file the first time a request reaches it.
//
// Startup never reports a source error. A file that does not parse stays out
// of the caches and reaches the interpreter when a request names it and one
// broken file below the root cannot stop the server coming up.
func precompile(files *handler, name string) {
	if !files.runnerOptions.Precompile {
		return
	}

	pass := runner.Precompiler{Root: files.root, Includes: files.includeCache}
	if files.flatstack {
		// Bytecode for a site running the interpreter is resident memory that
		// no request executes; only a site that reads it pays the flat compile.
		pass.Exprs = files.exprCache
	}
	cached := pass.Run()
	log.Printf("precompiled %s: %d files, %d bytes", name, cached, files.includeCache.Precompiled())
}
