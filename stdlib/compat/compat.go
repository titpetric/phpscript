// Package compat implements PHP language surface that a script expects to be
// there rather than a library it calls: functions whose behaviour is defined by
// what the interpreter does, not by what they compute.
//
// It is wired in by importing it, the way a program imports a database/sql
// driver:
//
//	import _ "github.com/titpetric/phpscript/stdlib/compat"
//
// stdlib does that for you (see stdlib/imports.go). A host that wants a
// different set builds its Runtime without stdlib.
//
// # Output buffering
//
// PHP's ob_* family redirects everything echo would emit into a stack of
// in-memory buffers, and template engines lean on it: rendering to a string is
// "start a buffer, include the compiled template, take the contents". That is
// what MiniTPL\Template::get() is built on.
//
//	ob_start();
//	include $compiled;
//	$html = ob_get_clean();
//
// The buffers are pushed onto the runtime with runner.PushOutput, so the
// capture covers everything that reaches Runtime.Output: echo, inline HTML, and
// builtins that emit text of their own, such as die() with a message.
//
// A function that reports "no buffer is active" returns false, as PHP does,
// rather than an empty string — the two are distinguishable with ===.
//
// # Regular expressions
//
// PHP's preg_* family is PCRE; Go's regexp is RE2, which cannot express
// backreferences or lookaround. regex.go compiles each pattern with whichever
// engine can express it, so both `\1` and a linear-time guarantee are
// available where they apply.
package compat

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the compatibility bindings (the ob_* family) to
// stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs symbols into the runtime. Each runtime gets its own buffer
// stack, so concurrent requests capture their own output.
func Register(rt *runner.Runtime) {
	registerRegex(rt)

	buffers := newBuffers(rt)

	// ob_start takes a callback and chunk size in PHP. Neither changes what a
	// template engine sees, so they are accepted and ignored.
	rt.RegisterFunc("ob_start", func(_ ...any) bool {
		buffers.push()
		return true
	})
	rt.RegisterFunc("ob_get_level", buffers.level)
	rt.RegisterFunc("ob_get_contents", func() any {
		contents, ok := buffers.contents()
		if !ok {
			return false
		}
		return contents
	})
	rt.RegisterFunc("ob_get_clean", func() any {
		contents, ok := buffers.pop(false)
		if !ok {
			return false
		}
		return contents
	})
	rt.RegisterFunc("ob_end_clean", func() bool {
		_, ok := buffers.pop(false)
		return ok
	})
	rt.RegisterFunc("ob_end_flush", func() bool {
		_, ok := buffers.pop(true)
		return ok
	})
	rt.RegisterFunc("ob_get_flush", func() any {
		contents, ok := buffers.pop(true)
		if !ok {
			return false
		}
		return contents
	})
}
