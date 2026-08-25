package runner

// ArithmeticError reports an operation PHP 8 rejects with the class of the same
// name: a shift by a negative number is the one this runtime raises. It names
// its PHP class, and a clause matches on that name, so `catch (ArithmeticError
// $e)` matches it exactly and `catch (Error $e)` matches it on the name's
// suffix, while `catch (Exception $e)` does not, as in PHP.
type ArithmeticError struct {
	Message string
}

// Error renders the message PHP carries on the same operation.
func (e *ArithmeticError) Error() string { return e.Message }

// ThrowableClass names the PHP class, implementing Throwable.
func (e *ArithmeticError) ThrowableClass() string { return "ArithmeticError" }

// DivisionByZeroError reports a division or modulo by zero that PHP rejects
// with the class of the same name: intdiv() and `%` raise it. It names its PHP
// class, so `catch (DivisionByZeroError $e)` matches it exactly and
// `catch (Error $e)` matches it on the name's suffix, as in PHP.
type DivisionByZeroError struct {
	Message string
}

// Error renders the message PHP carries for the same operation.
func (e *DivisionByZeroError) Error() string { return e.Message }

// ThrowableClass names the PHP class, implementing Throwable.
func (e *DivisionByZeroError) ThrowableClass() string { return "DivisionByZeroError" }
