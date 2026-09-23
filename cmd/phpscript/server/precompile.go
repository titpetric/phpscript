package server

import (
	"log"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
)

// sharedCaches puts a site's annotated files on the caches its file handler
// serves from. The routed endpoints and the document root read one source
// tree, so one precompile pass covers both instead of each parsing it again.
func sharedCaches(files *handler, options []annotations.Option) []annotations.Option {
	return append(options[:len(options):len(options)],
		annotations.WithIncludeCache(files.includeCache),
		annotations.WithExprCache(files.exprCache),
	)
}

// precompile parses a site's tree into its caches before the site serves a
// request, when runner.precompile asked for it. A site that did not is left
// lazy: it parses a file the first time a request reaches it.
//
// Startup is not where a source error is reported. A file that does not parse
// is left out of the caches and reaches the interpreter when a request names
// it, so one broken file below the root cannot stop the server coming up.
func precompile(files *handler, name string) {
	if !files.runnerOptions.Precompile {
		return
	}

	pass := runner.Precompiler{Root: files.root, Includes: files.includeCache}
	if files.flatstack {
		// Bytecode for a site running the interpreter is resident memory
		// nothing executes; the flat compile is only paid where it is read.
		pass.Exprs = files.exprCache
	}
	log.Printf("precompiled %s: %d files", name, pass.Run())
}
