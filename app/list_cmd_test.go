package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNameListWithoutPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	orig := make([]string, len(names))
	copy(orig, names)
	require.Equal(t, 3, len(names))
	sortSliceWithPrefix(names, "")
	require.Equal(t, names, orig)
}

func TestNameListWithPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	require.Equal(t, 3, len(names))
	sortSliceWithPrefix(names, "test")

	// should be test -> attest -> untested
	require.Equal(t, names[0], "test")
	require.Equal(t, names[1], "attest")
	require.Equal(t, names[2], "untested")
}

func TestNameListWithUnfoundPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	require.Equal(t, 3, len(names))
	sortSliceWithPrefix(names, "abracadabra")
	// should be attest -> test -> untested
	require.Equal(t, names[0], "attest")
	require.Equal(t, names[1], "test")
	require.Equal(t, names[2], "untested")
}

func TestNameListWithPrefixAndTieBreak(t *testing.T) {
	names := []string{"attest", "test", "untested", "testosterone"}
	require.Equal(t, 4, len(names))
	sortSliceWithPrefix(names, "test")
	// should be test -> testosterone -> attest -> untested
	require.Equal(t, names[0], "test")
	require.Equal(t, names[1], "testosterone")
	require.Equal(t, names[2], "attest")
	require.Equal(t, names[3], "untested")
}
