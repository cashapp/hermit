package errors

type Error struct {
	err  error
	code int
}

func (e *Error) ExitCode() int { return e.code }
func (e Error) Error() string  { return e.err.Error() }
