// Package regexp implements Regexp\Compile and Regexp\CompilePOSIX, Go's
// regexp package reached as PHP classes. A script that wants RE2 itself rather
// than the PCRE surface preg_* presents uses these; docs/bindings-regexp.md
// walks the binding from here to its fixtures.
//
// The constructors return *regexp.Regexp itself rather than a facade. Every
// exported method of that type is a regexp operation, so publishing the whole
// method set is the binding: $rx->find_string($s), $rx->split($s, -1),
// $rx->replace_all_string($s, $repl). The type is named Regexp, which is the
// class a script sees for the value and the name instanceof compares against.
//
// It is wired in by importing it; stdlib/imports.go does that.
package regexp

import (
	stdregexp "regexp"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the regexp bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the regexp compilation classes.
//
// Each is its package function behind a closure that names the argument, which
// is what the generated reference publishes as the PHP signature; registering
// the bare function would publish Go's own unnamed parameter.
func Register(rt *runner.Runtime) {
	// Regexp\Compile compiles $expr as an RE2 expression and throws when it does not parse; the value it builds carries every method Go's regexp.Regexp has.
	rt.RegisterConstructor("Regexp\\Compile", func(expr string) (*stdregexp.Regexp, error) {
		return stdregexp.Compile(expr)
	})
	// Regexp\CompilePOSIX compiles $expr as POSIX ERE, where a match is the leftmost-longest one rather than the leftmost one Perl syntax finds.
	rt.RegisterConstructor("Regexp\\CompilePOSIX", func(expr string) (*stdregexp.Regexp, error) {
		return stdregexp.CompilePOSIX(expr)
	})
}
