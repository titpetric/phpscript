package runner

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// RedeclareError is a second declaration of a function name that is already
// taken. PHP treats it as a fatal error; this runtime has one error path, so
// declareFunc returns it as a catchable RuntimeException, and it reaches a Go
// host as an error from Run.
//
// A named type and no bare error, because the two runtimes have to agree on
// it. A compile failure normally means the bytecode engine does not cover some
// form yet and the interpreter runs the program; this is a verdict on the
// program, which the interpreter would reach too, so flatstack raises it and
// falls back to no second opinion.
type RedeclareError struct {
	// Name is the function as a script spells it.
	Name string
	// At is the declaration declareFunc refuses.
	At FuncSite
	// Previous is the first declaration, zero when the name belongs to
	// a registered binding and not to a declaration in PHP source.
	Previous FuncSite
	// Builtin reports which of the two cases this is.
	Builtin bool
}

// Error writes PHP's fatal-error text for the same condition, with one
// difference by decision: the paths are the ones the script named, resolved
// against the root the shims bind to, where PHP prints host paths. A
// runtime serving scripts out of an fs.FS has no host path to print.
//
// A program a host assembled itself carries no file or spans, so Error omits
// the clauses that would name a location and never writes one empty.
func (e *RedeclareError) Error() string {
	message := fmt.Sprintf("Cannot redeclare function %s()", e.Name)
	if !e.Builtin {
		message += fmt.Sprintf(" (previously declared in %s)", e.Previous)
	}
	if e.At.File == "" {
		return message
	}
	if e.At.Line == 0 {
		return message + fmt.Sprintf(" in %s", e.At.File)
	}
	return message + fmt.Sprintf(" in %s on line %d", e.At.File, e.At.Line)
}

// GetMessage and GetCode are what a script reads off the caught value.
func (e *RedeclareError) GetMessage() string { return e.Error() }

// GetCode answers PHP's zero, since there is no error number to report.
func (e *RedeclareError) GetCode() int { return 0 }

// ThrowableClass names Exception, php having no class for a redeclaration.
func (e *RedeclareError) ThrowableClass() string { return "Exception" }

// declareFunc records a function declaration and refuses a name that is already
// taken, by an earlier declaration or by a registered binding.
//
// Only declarations hoist reaches get here, which is every declaration the
// runtime honours: a `function` written inside an `if` or a function body is
// not a statement hoist walks, and executing one is a no-op, so there is
// nothing to collide with. That is also why an `if (!function_exists('f'))`
// guard needs no special case here.
func (rt *Runtime) declareFunc(name string, site FuncSite) error {
	if previous, ok := rt.funcSites[name]; ok {
		return &RedeclareError{Name: name, At: site, Previous: previous}
	}
	// A name the runtime already answers to, with no site recorded for it, is
	// one of the bindings stdlib registered. PHP refuses those too, and says
	// less about them: there is no earlier declaration to point at.
	if _, taken := rt.lookupFunc(name); taken {
		return &RedeclareError{Name: name, At: site, Builtin: true}
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
