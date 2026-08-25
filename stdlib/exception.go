package stdlib

import "github.com/titpetric/phpscript/runner"

// Exception holds an error code and error message.
//
// Every PHP throwable class is this one Go type. Class records the name the
// script constructed, so get_class() can answer it and a catch clause can
// filter on it; no two of them stand in a subclass relation, because phpscript
// has no inheritance. See docs/design.md.
type Exception struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Class   string `json:"class"`
}

// ThrowableClass returns the PHP class the exception was constructed as,
// implementing runner.Throwable.
func (e *Exception) ThrowableClass() string {
	if e.Class == "" {
		return "Exception"
	}
	return e.Class
}

// NewException will return a new allocation for an exception.
// A nil error is returned so a new exception is not thrown immediately
// whenever a `new Exception` call is made in the runtime.
//
// The exception is returned as a pointer. Returning the struct by value boxed
// a copy into the interface the VM holds, which costs the same allocation but
// loses reference semantics: the boxed copy is not addressable, so
// `$e->message = "..."` could not write to it and a method could not mutate
// the instance the script holds.
func NewException(message string, code int) (*Exception, error) {
	return &Exception{
		Message: message,
		Code:    code,
		Class:   "Exception",
	}, nil
}

// newThrowable returns the constructor for one PHP throwable class. Each class
// builds the same value and differs only in the name it records, which is what
// a catch clause filters on and what get_class() reports.
func newThrowable(class string) func(string, int) (*Exception, error) {
	return func(message string, code int) (*Exception, error) {
		return &Exception{
			Message: message,
			Code:    code,
			Class:   class,
		}, nil
	}
}

// GetCode returns the error code of the exception.
func (e *Exception) GetCode() int {
	return e.Code
}

// GetMessage returns the error message of the exception.
func (e *Exception) GetMessage() string {
	return e.Message
}

// Error is implemented to satisfy an error interface.
func (e *Exception) Error() string {
	return e.Message
}

// splExceptions are the SPL and Error class names a PHP library throws. None of
// them adds behaviour over Exception, so they all construct the same value with
// a different Class, which is what makes `throw new \InvalidArgumentException(...)`
// work rather than fail on an undefined class.
var splExceptions = []string{
	"ErrorException",
	"RuntimeException",
	"LogicException",
	"InvalidArgumentException",
	"DomainException",
	"LengthException",
	"OutOfRangeException",
	"OutOfBoundsException",
	"RangeException",
	"OverflowException",
	"UnderflowException",
	"UnexpectedValueException",
	"BadFunctionCallException",
	"BadMethodCallException",
	"JsonException",
	"Error",
	"TypeError",
	"ValueError",
	"ArithmeticError",
	"DivisionByZeroError",
	"ArgumentCountError",
}

func registerExceptions(rt *runner.Runtime) {
	// Exception is PHP's base exception class; the SPL exception and Error classes are the same Go type carrying a different class name, so a catch clause filters on the name rather than on a subclass relation.
	rt.RegisterConstructor("Exception", NewException)
	for _, class := range splExceptions {
		rt.RegisterConstructor(class, newThrowable(class))
	}
}
