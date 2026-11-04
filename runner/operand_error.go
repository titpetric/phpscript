package runner

// OperandError reports an operation between two typed go values of different
// widths. go refuses `int8 + int16` outright, this refuses the same pair, and
// php words a refusal between operand types "Unsupported operand types" and
// raises TypeError for it: `catch (TypeError $e)` sees this one.
//
// An explicit conversion is what a script writes instead. php has no
// promotion rule between widths to borrow, and inventing one here would read
// as go's arithmetic while answering with neither language's result.
type OperandError struct {
	Message string
}

// Error renders the message php carries for the same pair of operands.
func (e *OperandError) Error() string { return e.Message }

// ThrowableClass names the PHP class, implementing Throwable.
func (e *OperandError) ThrowableClass() string { return "TypeError" }
