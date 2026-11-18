package stdlib

import (
	"github.com/titpetric/phpscript/runner"
)

// Exception holds an error code and error message.
//
// Every PHP throwable class is this Go type. Class records the constructed
// name: get_class() answers the name; a catch clause filters on the name. No
// subclass relation joins two exceptions; phpscript has no inheritance. See
// docs/design.md.
type Exception struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Class   string `json:"class"`
}

// ThrowableClass returns the PHP class the exception declares,
// implementing runner.Throwable.
func (e *Exception) ThrowableClass() string {
	if e.Class == "" {
		return "Exception"
	}
	return e.Class
}

// NewException returns an exception and `new Exception` does not throw. The
// pointer keeps `$e->message = "..."` writable: a boxed struct is not addressable.
func NewException(message string, code int) (*Exception, error) {
	return &Exception{
		Message: message,
		Code:    code,
		Class:   "Exception",
	}, nil
}

// newThrowable returns a constructor for one throwable class, recording that class name.
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

// Error returns the message and *Exception is a Go error.
func (e *Exception) Error() string {
	return e.Message
}

// splExceptions are SPL exception class names a PHP library throws.
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
}

// splErrors are Error class names a PHP file may throw.
var splErrors = []string{
	"Error",
	"TypeError",
	"ValueError",
	"ArithmeticError",
	"DivisionByZeroError",
	"ArgumentCountError",
}

func registerExceptions(rt *runner.Runtime) {
	// Exception is PHP's base exception class; the SPL exception and Error
	// classes are the same Go type under a different class name and a catch
	// clause filters on the name and not on a subclass relation.
	rt.RegisterConstructor("Exception", NewException)
	for _, class := range splExceptions {
		rt.RegisterConstructor(class, newThrowable(class))
	}
}

func registerErrors(rt *runner.Runtime) {
	for _, class := range splErrors {
		rt.RegisterConstructor(class, newThrowable(class))
	}
}
