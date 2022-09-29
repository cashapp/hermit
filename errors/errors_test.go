package errors

import (
	"fmt"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestLineAndFormatting(t *testing.T) {
	err := New("an error")
	wrapErr := Wrap(err, "another error")
	assert.Equal(t, `an error`, fmt.Sprintf("%s", err))
	assert.Equal(t, `"an error"`, fmt.Sprintf("%q", err))
	assert.Equal(t, `errors/errors_test.go:11: an error`, fmt.Sprintf("%+v", err))
	assert.Equal(t, `another error: an error`, fmt.Sprintf("%s", wrapErr))
	assert.Equal(t, `errors/errors_test.go:12: another error: errors/errors_test.go:11: an error`, fmt.Sprintf("%+v", wrapErr))
}
