package app

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNameListWithoutPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	nl := NameList{nl: names}
	require.Equal(t, 3, nl.Len())
	sort.Sort(nl)
	require.Equal(t, names, nl.nl)
}

func TestNameListWithPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	nl := NameList{nl: names, prefix: "test"}
	require.Equal(t, 3, nl.Len())
	t.Log(nl.nl)
	sort.Sort(nl)
	t.Log(nl.nl)
	// should be test -> attest -> untested
	require.Equal(t, nl.nl[0], "test")
	require.Equal(t, nl.nl[1], "attest")
	require.Equal(t, nl.nl[2], "untested")
}

func TestNameListWithUnfoundPrefix(t *testing.T) {
	names := []string{"attest", "test", "untested"}
	nl := NameList{nl: names, prefix: "abracadabra"}
	require.Equal(t, 3, nl.Len())
	t.Log(nl.nl)
	sort.Sort(nl)
	t.Log(nl.nl)
	// should be test -> attest -> untested
	require.Equal(t, nl.nl[0], "attest")
	require.Equal(t, nl.nl[1], "test")
	require.Equal(t, nl.nl[2], "untested")
}
