package apperr

type Error struct {
	Code    string         `json:"code"`
	Status  int            `json:"-"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (err *Error) Error() string { return err.Code }

func New(status int, code, message string) *Error {
	return &Error{Code: code, Status: status, Message: message}
}
