package errors

type ExitError struct {
	err  error
	code int
}

func (e *ExitError) Error() string { return e.err.Error() }
func (e *ExitError) Unwrap() error { return e.err }
func (e *ExitError) ExitCode() int { return e.code }

func WithExitCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &ExitError{err: err, code: code}
}
