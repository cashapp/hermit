package errors

import (
	"errors"

	"github.com/alecthomas/kong"
)

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

// ExitCodeFromError returns the exit code for the given error.
// If err implements kong.ExitCoder, ExitCode is used.
// Otherwise, returns 0 if err == nil, or 1 for a generic failure.
func ExitCodeFromError(err error) int {
	var ec kong.ExitCoder
	switch {
	case errors.As(err, &ec):
		return ec.ExitCode()
	case err == nil:
		return 0
	default:
		return 1
	}
}
