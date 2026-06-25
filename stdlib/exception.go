package stdlib

type Exception struct {
	Message string
	Code    int
}

func NewException(message string, code int) error {
	return &Exception{
		Message: message,
		Code:    code,
	}
}

func (e *Exception) GetCode() int {
	return e.Code
}

func (e *Exception) GetMessage() string {
	return e.Message
}

func (e *Exception) Error() string {
	return e.Message
}
