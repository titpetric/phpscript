package compat

import (
	"io"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// buffers is one runtime's stack of output buffering levels, typed so ob_get_contents can read them.
type buffers struct {
	rt    *runner.Runtime
	stack []*strings.Builder
}

func newBuffers(rt *runner.Runtime) *buffers {
	return &buffers{rt: rt}
}

// push starts a buffering level, backing ob_start.
func (b *buffers) push() {
	buffer := &strings.Builder{}
	b.stack = append(b.stack, buffer)
	b.rt.PushOutput(buffer)
}

// level reports how many levels are active, backing ob_get_level.
func (b *buffers) level() int { return len(b.stack) }

// reset drops every buffering level a program left open, the runtime resetting its side too.
func (b *buffers) reset() {
	clear(b.stack)
	b.stack = b.stack[:0]
}

// contents returns the innermost buffer without ending it, false with no buffer active.
func (b *buffers) contents() (string, bool) {
	if len(b.stack) == 0 {
		return "", false
	}
	return b.stack[len(b.stack)-1].String(), true
}

// pop ends the innermost level and returns its contents, writing them on when flush is true.
func (b *buffers) pop(flush bool) (string, bool) {
	if len(b.stack) == 0 {
		return "", false
	}
	last := len(b.stack) - 1
	contents := b.stack[last].String()
	b.stack = b.stack[:last]
	b.rt.PopOutput()
	if flush {
		// Writing after the pop targets the enclosing level and nested buffers
		// compose. WriteString, not Write: a []byte conversion here would copy
		// the whole page every time a template engine flushes one.
		if _, err := io.WriteString(b.rt.Output(), contents); err != nil {
			return contents, true
		}
	}
	return contents, true
}
