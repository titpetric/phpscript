package runner

import (
	"fmt"
)

// FuncSite records where a script declares a user function. The site turns a
// second declaration of a name into a message worth reading: PHP's fatal error
// names the first site. A script with two declarations of a function got there
// by including a file twice; the first site alone says which file.
type FuncSite struct {
	File string
	Line int
}

// String renders the site the way PHP writes it in the fatal error, and the
// file alone when the declaration came from a program without line
// information, as a host that built the AST passes in.
func (s FuncSite) String() string {
	if s.File == "" {
		return "an earlier declaration"
	}
	if s.Line == 0 {
		return s.File
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}
