package runner

import (
	"fmt"
)

// FuncSite is where a user function was declared. It is what turns the second
// declaration of a name into a message worth reading: PHP's fatal error names
// the first site, and a script carrying two declarations of the same
// function usually got there by including a file twice, in which case the first
// site is the only thing that says which file.
type FuncSite struct {
	File string
	Line int
}

// String renders the site the way PHP writes it in the fatal error, and the
// file alone when the declaration came from a program with no line information,
// as a host that built the AST itself passes in.
func (s FuncSite) String() string {
	if s.File == "" {
		return "an earlier declaration"
	}
	if s.Line == 0 {
		return s.File
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}
