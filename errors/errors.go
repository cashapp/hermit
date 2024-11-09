package errors

import (
	"errors" // nolint: depguard
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cashapp/hermit/util/debug"
)

type herr struct {
	cause error
	file  string
	line  int
	msg   string
}

func (h *herr) Error() string {
	return h.format(debug.Flags.ErrorTrace)
}

func (h *herr) Unwrap() error {
	return h.cause
}

func (h *herr) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			fmt.Fprint(s, h.format(true))
		} else {
			fmt.Fprint(s, h.Error())
		}
	case 's':
		fmt.Fprint(s, h.format(debug.Flags.ErrorTrace))
	case 'q':
		fmt.Fprintf(s, "%q", h.format(debug.Flags.ErrorTrace))
	}
}

func (h *herr) format(trace bool) string {
	var msg string
	if trace {
		msg += fmt.Sprintf("%s:%d", h.file, h.line)
		if h.msg != "" {
			msg += ": " + h.msg
		}
	} else {
		msg += h.msg
	}
	if h.cause != nil {
		if msg != "" {
			msg += ": "
		}
		if trace {
			msg += fmt.Sprintf("%+v", h.cause)
		} else {
			msg += h.cause.Error()
		}
	}
	return msg
}

var pkgPrefix = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file)) + "/"
}()

func newErr(cause error, msg string) error {
	_, file, line, _ := runtime.Caller(2)
	file = strings.TrimPrefix(file, pkgPrefix)
	return &herr{cause: cause, file: file, line: line, msg: msg}
}

// New creates a new error.
func New(message string) error {
	return newErr(nil, message)
}

// Errorf creates a new error using fmt.Sprintf().
func Errorf(format string, args ...interface{}) error {
	return newErr(nil, fmt.Sprintf(format, args...))
}

// Wrap chains a new error to "err" if it is not nil.
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return newErr(err, message)
}

// Wrapf chains a new fmt.Sprintf() formatted error to "err" if "err" is not nil.
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return newErr(err, fmt.Sprintf(format, args...))
}

// Is mirrors the stdlib errors.Is function.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// Unwrap aliases the stdlib errors.Unwrap function.
func Unwrap(err error) error {
	return errors.Unwrap(err)
}

// WithStack chains source location information to an error if "err" is not nil.
func WithStack(err error) error {
	if err == nil {
		return nil
	}
	return newErr(err, "")
}

func Join(errs ...error) error {
	return errors.Join(errs...)
}
