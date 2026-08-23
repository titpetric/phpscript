package runner

// ArithmeticError reports an operation PHP 8 rejects with the class of the same
// name: a shift by a negative number is the one this runtime raises. The Go
// type name is the class name a script sees (see errorClassName), so
// `catch (ArithmeticError $e)` matches it and `catch (Exception $e)` does not,
// exactly as in PHP.
type ArithmeticError struct {
	Message string
}

// Error renders the message PHP carries on the same operation.
func (e *ArithmeticError) Error() string { return e.Message }
