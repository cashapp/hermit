package app

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestNameListWithoutPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	orig := make([]string, len(names))
	copy(orig, names)
	assert.Equal(t, 3, len(names))
	sortSliceWithPrefix(names, "")
	assert.Equal(t, names, orig)
}

func TestNameListWithPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	assert.Equal(t, 3, len(names))
	sortSliceWithPrefix(names, "test")

	// should be test -> attest -> untested
	assert.Equal(t, names[0], "test")
	assert.Equal(t, names[1], "attest")
	assert.Equal(t, names[2], "untested")
}

func TestNameListWithUnfoundPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	assert.Equal(t, 3, len(names))
	sortSliceWithPrefix(names, "abracadabra")
	// should be attest -> test -> untested
	assert.Equal(t, names[0], "attest")
	assert.Equal(t, names[1], "test")
	assert.Equal(t, names[2], "untested")
}

func TestNameListWithPrefixAndTieBreak(t *testing.T) {
	names := []string{"attest", "test", "untested", "testosterone"}
	assert.Equal(t, 4, len(names))
	sortSliceWithPrefix(names, "test")
	// should be test -> testosterone -> attest -> untested
	assert.Equal(t, names[0], "test")
	assert.Equal(t, names[1], "testosterone")
	assert.Equal(t, names[2], "attest")
	assert.Equal(t, names[3], "untested")
}
