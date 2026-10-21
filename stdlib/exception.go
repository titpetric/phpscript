package stdlib

import (
	"github.com/titpetric/phpscript/runner"
)

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

// ThrowableClass returns the PHP class the exception declares,
// implementing runner.Throwable.
func (e *Exception) ThrowableClass() string {
	if e.Class == "" {
		return "Exception"
	}
	return e.Class
}

// NewException returns an exception and a nil error, so `new Exception` does
// not throw. The pointer is what keeps `$e->message = "..."` writable: a
// struct boxed into the VM's interface is not addressable.
func NewException(message string, code int) (*Exception, error) {
	return &Exception{
		Message: message,
		Code:    code,
		Class:   "Exception",
	}, nil
}

// newThrowable returns the constructor for one PHP throwable class. Each class
// builds the same value and differs only in the name it records: a catch
// clause filters on that name and get_class() reports it.
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

// Error returns the message, so *Exception is a Go error.
func (e *Exception) Error() string {
	return e.Message
}

// splExceptions are the SPL and Error class names a PHP library throws. None of
// them adds behaviour over Exception, so they all construct the same value with
// a different Class, so `throw new \InvalidArgumentException(...)` runs
// where an undefined class would otherwise fail it.
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
	// Exception is PHP's base exception class; the SPL exception and Error
	// classes are the same Go type carrying a different class name, so a
	// catch clause filters on the name and not on a subclass relation.
	rt.RegisterConstructor("Exception", NewException)
	for _, class := range splExceptions {
		rt.RegisterConstructor(class, newThrowable(class))
	}
}
