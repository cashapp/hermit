package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLineAndFormatting(t *testing.T) {
	err := New("an error")
	wrapErr := Wrap(err, "another error")
	require.Equal(t, `an error`, fmt.Sprintf("%s", err))
	require.Equal(t, `"an error"`, fmt.Sprintf("%q", err))
	require.Equal(t, `errors/errors_test.go:11: an error`, fmt.Sprintf("%+v", err))
	require.Equal(t, `another error: an error`, fmt.Sprintf("%s", wrapErr))
	require.Equal(t, `errors/errors_test.go:12: another error: errors/errors_test.go:11: an error`, fmt.Sprintf("%+v", wrapErr))
}
