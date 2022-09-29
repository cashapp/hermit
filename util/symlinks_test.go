package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestSymlinks(t *testing.T) {
	pwd, err := os.Getwd()
	assert.NoError(t, err)
	expected := []string{
		filepath.Join(pwd, "testdata/three"),
		filepath.Join(pwd, "testdata/sub/two"),
		filepath.Join(pwd, "testdata/one"),
		filepath.Join(pwd, "testdata/dest"),
	}
	actual, err := ResolveSymlinks("testdata/three")
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}
