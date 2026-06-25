package stdlib

// Exception holds an error code and error message.
type Exception struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewException will return a new allocation for an exception.
// A nil error is returned so a new exception is not thrown immediately
// whenever a `new Exception` call is made in the runtime.
func NewException(message string, code int) (Exception, error) {
	return Exception{
		Message: message,
		Code:    code,
	}, nil
}

// GetCode returns the error code of the exception.
func (e Exception) GetCode() int {
	return e.Code
}

// GetMessage returns the error message of the exception.
func (e Exception) GetMessage() string {
	return e.Message
}

// Error is implemented to satisfy an error interface.
func (e Exception) Error() string {
	return e.Message
}
