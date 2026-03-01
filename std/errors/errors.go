package errors

type errorString struct {
	s string
}

func (e *errorString) Error() string {
	return e.s
}

func New(text string) error {
	return &errorString{s: text}
}

type unwrapper interface {
	Unwrap() error
}

func Unwrap(err error) error {
	_ = err
	return nil
}
