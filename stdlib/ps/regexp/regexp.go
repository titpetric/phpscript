// Package regexp implements Regexp\Compile and Regexp\CompilePOSIX, Go's
// regexp package reached as PHP classes. A script that wants RE2 itself uses
// these; preg_* presents the PCRE surface. docs/bindings-regexp.md walks the
// binding from here to its fixtures.
//
// The constructors return *regexp.Regexp itself and no facade. Every
// exported method of that type is a regexp operation and publishing the whole
// method set is the binding: $rx->find_string($s), $rx->split($s, -1),
// $rx->replace_all_string($s, $repl). The runtime publishes the type as Regexp,
// the class a script sees for the value and the name instanceof compares
// against.
//
// stdlib/imports.go wires it in by importing it.
package regexp

import (
	stdregexp "regexp"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the regexp bindings to stdlib.Register.
func init() {
	runner.RegisterArea(`Runtime\PS\Regexp`, Register)
}

// Register installs the regexp compilation classes.
//
// Each is its package function behind a closure that names the argument: the
// generated reference publishes that name as the PHP signature.
func Register(rt *runner.Runtime) {
	// Regexp\Compile compiles $expr as an RE2 expression and throws when it
	// does not parse; the value it builds holds every method Go's
	// regexp.Regexp has.
	rt.RegisterConstructor("Regexp\\Compile", func(expr string) (*stdregexp.Regexp, error) {
		return stdregexp.Compile(expr)
	})
	// Regexp\CompilePOSIX compiles $expr as POSIX ERE, where a match is the
	// leftmost-longest one and not the leftmost one Perl syntax finds.
	rt.RegisterConstructor("Regexp\\CompilePOSIX", func(expr string) (*stdregexp.Regexp, error) {
		return stdregexp.CompilePOSIX(expr)
	})
}
