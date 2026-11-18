package runner

// RuntimeException represents a PHP RuntimeException raised by the runner.
type RuntimeException struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Error is the message a Go caller reads.
func (e *RuntimeException) Error() string {
	return e.Message
}

// GetMessage answers the message to a script calling `getMessage`.
func (e *RuntimeException) GetMessage() string {
	return e.Message
}

// GetCode lets a script read the exception's numeric status, which PHP spells `getCode`.
func (e *RuntimeException) GetCode() int {
	return e.Code
}

// ThrowableClass names the PHP class a catch clause filters on.
func (e *RuntimeException) ThrowableClass() string { return "RuntimeException" }

// NewRuntimeException creates a new RuntimeException instance.
func NewRuntimeException(message string, code int) *RuntimeException {
	return &RuntimeException{
		Message: message,
		Code:    code,
	}
}
