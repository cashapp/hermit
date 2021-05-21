package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymlinks(t *testing.T) {
	pwd, err := os.Getwd()
	require.NoError(t, err)
	expected := []string{
		filepath.Join(pwd, "testdata/three"),
		filepath.Join(pwd, "testdata/sub/two"),
		filepath.Join(pwd, "testdata/one"),
		filepath.Join(pwd, "testdata/dest"),
	}
	actual, err := ResolveSymlinks("testdata/three")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
