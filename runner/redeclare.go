package runner

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// FuncSite is where a user function was declared. It is what turns the second
// declaration of a name into a message worth reading: PHP's fatal error names
// the first site, and a script that ends up with two declarations of the same
// function usually got there by including a file twice, in which case the first
// site is the only thing that says which file.
type FuncSite struct {
	File string
	Line int
}

// String renders the site the way PHP writes it in the fatal error, and the
// file alone when the declaration came from a program with no line information,
// which is what a host that built the AST itself hands over.
func (s FuncSite) String() string {
	if s.File == "" {
		return "an earlier declaration"
	}
	if s.Line == 0 {
		return s.File
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}

// RedeclareError is a second declaration of a function name that is already
// taken. PHP treats it as a fatal error; this runtime has one error path, so it
// is raised as a catchable RuntimeException and reaches a Go host as an error
// from Run.
//
// It is a type rather than a bare error because the two runtimes have to agree
// on it. A compile failure normally means the bytecode engine does not cover
// some form yet and the interpreter runs the program instead; this is not that.
// It is a verdict on the program, which the interpreter would reach too, so
// flatstack raises it rather than falling back to a second opinion.
type RedeclareError struct {
	// Name is the function as a script spells it.
	Name string
	// Previous is where it was declared first, zero when the name belongs to
	// a registered binding rather than to a declaration in PHP source.
	Previous FuncSite
	// Builtin reports which of the two cases this is.
	Builtin bool
}

func (e *RedeclareError) Error() string {
	if e.Builtin {
		return fmt.Sprintf("Cannot redeclare function %s()", e.Name)
	}
	return fmt.Sprintf("Cannot redeclare function %s() (previously declared in %s)", e.Name, e.Previous)
}

// GetMessage and GetCode are what a script reads off the caught value.
func (e *RedeclareError) GetMessage() string { return e.Error() }

// GetCode answers PHP's zero, since there is no error number to report.
func (e *RedeclareError) GetCode() int { return 0 }

// ThrowableClass names the class a catch clause filters on. Exception rather
// than a name of its own: PHP has no class for this at all, because there it is
// a compile-time fatal that no catch can reach, so there is no spelling to
// match and Exception is the clause a script writing one would write.
func (e *RedeclareError) ThrowableClass() string { return "Exception" }

// declareFunc records a function declaration and refuses a name that is already
// taken, by an earlier declaration or by a registered binding.
//
// Only declarations hoist reaches get here, which is every declaration the
// runtime honours: a `function` written inside an `if` or a function body is
// not a statement hoist walks, and executing it is a no-op, so there is nothing
// to collide with. That is also why an `if (!function_exists('f'))` guard needs
// no special case here.
func (rt *Runtime) declareFunc(name string, site FuncSite) error {
	if previous, ok := rt.funcSites[name]; ok {
		return &RedeclareError{Name: name, Previous: previous}
	}
	// A name the runtime already answers to, with no site recorded for it, is
	// one of the bindings stdlib registered. PHP refuses those too, and says
	// less about them: there is no file to point at.
	if _, taken := rt.lookupFunc(name); taken {
		return &RedeclareError{Name: name, Builtin: true}
	}
	rt.funcSites[name] = site
	return nil
}

// declSite reads the source span a program recorded for a statement. A program
// a host assembled itself carries no spans, so the line is simply absent rather
// than wrong.
func declSite(prog *model.Program, stmt model.Stmt, filename string) FuncSite {
	return FuncSite{File: filename, Line: prog.SourceSpans[stmt].Start}
}
