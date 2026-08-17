package runner

import (
	"strings"
)

// Output buffering. PHP's ob_* family redirects everything echo would emit into
// a stack of in-memory buffers, and template engines lean on it: rendering to a
// string is "start a buffer, include the compiled template, take the contents".
//
// The runtime writes all script output through Output(), so buffering is a
// matter of returning the innermost buffer from there instead of the writer the
// runtime was constructed with.

// PushOutputBuffer starts a new output buffering level, backing ob_start.
func (rt *Runtime) PushOutputBuffer() {
	rt.obStack = append(rt.obStack, &strings.Builder{})
}

// OutputBufferLevel reports how many buffering levels are active, backing
// ob_get_level.
func (rt *Runtime) OutputBufferLevel() int { return len(rt.obStack) }

// OutputBufferContents returns the innermost buffer's contents without ending
// it, backing ob_get_contents. The second result is false when no buffer is
// active, which PHP reports as a false return.
func (rt *Runtime) OutputBufferContents() (string, bool) {
	if len(rt.obStack) == 0 {
		return "", false
	}
	return rt.obStack[len(rt.obStack)-1].String(), true
}

// PopOutputBuffer ends the innermost buffering level and returns its contents.
// When flush is true the contents are written to the enclosing level (or the
// real output) the way ob_end_flush does; otherwise they are discarded, as
// ob_end_clean does. The second result is false when no buffer is active.
func (rt *Runtime) PopOutputBuffer(flush bool) (string, bool) {
	if len(rt.obStack) == 0 {
		return "", false
	}
	last := len(rt.obStack) - 1
	contents := rt.obStack[last].String()
	rt.obStack = rt.obStack[:last]
	if flush {
		// Writing after the pop targets the enclosing level, which is what
		// makes nested buffers compose.
		if _, err := rt.Output().Write([]byte(contents)); err != nil {
			return contents, true
		}
	}
	return contents, true
}
