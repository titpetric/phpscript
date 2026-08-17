package runner

import (
	"io"
)

// Output redirection. The runtime writes all script output through Output(), so
// capturing it is a matter of returning something else from there: push a
// writer, and everything echo would emit lands in it until the matching pop.
//
// The stack is here because Output() is the runtime's own seam, but nothing
// here knows what the captured text is for. PHP's ob_* family is built on top
// of it in stdlib/compat, which pushes the buffers it hands back to a script.

// PushOutput redirects script output to w until the matching PopOutput. Pushes
// nest: the innermost writer receives the output, so a captured region can
// itself capture.
func (rt *Runtime) PushOutput(w io.Writer) {
	if w == nil {
		// Every push has to leave something to pop. Dropping the level instead
		// would make the next PopOutput unstack the caller's enclosing writer,
		// which is worse than discarding the text.
		w = io.Discard
	}
	rt.outStack = append(rt.outStack, w)
}

// PopOutput ends the innermost redirection, reporting whether one was active.
// Output resumes going to the enclosing writer, which is what makes nested
// captures compose.
func (rt *Runtime) PopOutput() bool {
	if len(rt.outStack) == 0 {
		return false
	}
	rt.outStack = rt.outStack[:len(rt.outStack)-1]
	return true
}

// OutputDepth reports how many redirections are active.
func (rt *Runtime) OutputDepth() int { return len(rt.outStack) }
