// Package compat implements the PHP language surface a script uses
// directly, without any library it calls: functions whose behaviour the
// interpreter fixes, whatever their computed result.
//
// Importing it wires it in, the way a program imports a database/sql
// driver:
//
//	import _ "github.com/titpetric/phpscript/stdlib/ps/compat"
//
// stdlib does that for you (see stdlib/imports.go). A host needing a
// different set builds its Runtime without stdlib.
//
// # Output buffering
//
// PHP's ob_* family redirects everything echo would emit into a stack of
// in-memory buffers. Template engines lean on it: rendering to a string is
// "start a buffer, include the compiled template, take the contents".
// MiniTPL\Template::get() builds on exactly that.
//
//	ob_start();
//	include $compiled;
//	$html = ob_get_clean();
//
// ob_start pushes a buffer onto the runtime with runner.PushOutput. The
// capture covers everything that reaches Runtime.Output: echo, inline HTML and
// builtins that emit text directly, such as die() with a message.
//
// A function that reports "no buffer is active" returns false, as PHP does.
// It returns no empty string; the two are distinguishable with ===.
//
// # Regular expressions
//
// PHP's preg_* family is PCRE; Go's regexp is RE2, which cannot express
// backreferences or lookaround. regex.go compiles each pattern with the
// engine that can express it. Both `\1` backreferences and linear-time
// matching are available where they apply.
package compat

import (
	"github.com/titpetric/phpscript/runner"
)

// init registers the compatibility area with the runtime's area table.
func init() {
	runner.RegisterArea(`Runtime\PS\Compat`, Register)
}

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	registerRegex(rt)
	registerDatetime(rt)
	RegisterBuffers(rt)
}

// RegisterBuffers installs the ob_* output buffering functions. Each runtime
// gets a separate buffer stack, and concurrent requests capture their own output.
func RegisterBuffers(rt *runner.Runtime) {
	buffers := newBuffers(rt)
	// A reset between programs drops the levels with the redirections they
	// mirror. A reused runtime answers ob_get_level with the levels this
	// program opened, never a level the last one left.
	rt.OnReset(buffers.reset)

	// ob_start takes a callback and chunk size in PHP. Neither changes what a
	// template engine sees and ob_start accepts and ignores both.
	rt.RegisterFunc("ob_start", func(_ ...any) bool {
		buffers.push()
		return true
	})
	// ob_get_level returns the number of active output buffers.
	rt.RegisterFunc("ob_get_level", buffers.level)
	// ob_get_contents returns the active buffer's contents without closing it.
	rt.RegisterFunc("ob_get_contents", func() any {
		contents, ok := buffers.contents()
		if !ok {
			return false
		}
		return contents
	})
	// ob_get_clean closes the active buffer and returns its contents.
	rt.RegisterFunc("ob_get_clean", func() any {
		contents, ok := buffers.pop(false)
		if !ok {
			return false
		}
		return contents
	})
	// ob_end_clean closes the active buffer and discards its contents.
	rt.RegisterFunc("ob_end_clean", func() bool {
		_, ok := buffers.pop(false)
		return ok
	})
	// ob_end_flush closes the active buffer and writes its contents to the output below it.
	rt.RegisterFunc("ob_end_flush", func() bool {
		_, ok := buffers.pop(true)
		return ok
	})
	// ob_get_flush closes the active buffer, writes its contents to the
	// output below it, and returns them.
	rt.RegisterFunc("ob_get_flush", func() any {
		contents, ok := buffers.pop(true)
		if !ok {
			return false
		}
		return contents
	})
}
