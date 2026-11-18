package runner

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// RedeclareError is a second declaration of a function name already taken. PHP
// treats it as a fatal error. This runtime has a single error path, and
// declareFunc returns the error as a catchable RuntimeException. It reaches a
// Go host as an error from Run.
//
// A named type, not a bare error: the two runtimes have to agree on the type.
// A compile failure normally means the bytecode engine does not cover some
// form yet. The interpreter runs the program. This error is a verdict on the
// program, which the interpreter would reach too. flatstack raises the error
// and decides alone.
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

// Error writes PHP's fatal-error text, with one difference: the paths are the
// ones the script named, where PHP prints host paths. A program a host
// assembled itself has no file and Error omits the clauses that would name a
// location.
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

// GetMessage and GetCode supply the values a script reads off the caught value.
func (e *RedeclareError) GetMessage() string { return e.Error() }

// GetCode answers PHP's zero, since there is no error number to report.
func (e *RedeclareError) GetCode() int { return 0 }

// ThrowableClass names Exception, because php has no class for a redeclaration.
func (e *RedeclareError) ThrowableClass() string { return "Exception" }

// declareFunc records a declaration and refuses a name already taken.
func (rt *Runtime) declareFunc(name string, site FuncSite) error {
	if previous, ok := rt.funcSites[name]; ok {
		return &RedeclareError{Name: name, At: site, Previous: previous}
	}
	// A name the runtime already answers to has no recorded site; a binding
	// stdlib registered owns it. PHP refuses those too. It says less about
	// them: there is no earlier declaration to point at.
	if _, taken := rt.lookupFunc(name); taken {
		return &RedeclareError{Name: name, At: site, Builtin: true}
	}
	rt.funcSites[name] = site
	return nil
}

// declSite reads the source span a program recorded, zero when absent.
func declSite(prog *model.Program, stmt model.Stmt, filename string) FuncSite {
	return FuncSite{File: filename, Line: prog.SourceSpans[stmt].Start}
}
