package manifest

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReferenceMatch(t *testing.T) {
	require.True(t, ParseReference("go-1.13").Match(ParseReference("go-1.13.7")))
	require.False(t, ParseReference("go-1.12").Match(ParseReference("go-1.13.7")))
	require.False(t, ParseReference("protoc-3.15.0-square-1.0").Match(ParseReference("protoc-3.15.0")))
	require.False(t, ParseReference("protoc-3.15.0-square-1.0").Match(ParseReference("protoc-3.15.0-square-1")))
	require.True(t, ParseReference("protoc-3.15.0-square-1").Match(ParseReference("protoc-3.15.0-square-1.0")))
	require.True(t, ParseReference("protoc-3.15.0-square-1.0").Match(ParseReference("protoc-3.15.0-square-1.0")))
}

func TestParseReferences(t *testing.T) {
	tests := []struct {
		version    string
		parts      string
		prerelease string
		metadata   string
	}{
		{"1.2.3", "1.2.3", "", ""},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			v := ParseVersion(test.version)
			require.Equal(t, test.parts, strings.Join(v.Components(), "."))
			require.Equal(t, test.prerelease, v.Prerelease())
			require.Equal(t, test.metadata, v.Metadata())
		})
	}
}

func TestParseVersions(t *testing.T) {
	tests := []struct {
		version    string
		parts      string
		prerelease string
		metadata   string
	}{
		{"1.2.3", "1.2.3", "", ""},
		{"1.5.1-kotlin.3", "1.5.1", "kotlin.3", ""},
		{"11.0.10_9", "11.0.10.9", "", ""},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			v := ParseVersion(test.version)
			require.Equal(t, test.parts, strings.Join(v.Components(), "."))
			require.Equal(t, test.prerelease, v.Prerelease())
			require.Equal(t, test.metadata, v.Metadata())
		})
	}
}

func TestSortVersions(t *testing.T) {
	v0 := ParseVersion("1.13")
	v1 := ParseVersion("1.13.5")
	v2 := ParseVersion("1.13.4")
	v3 := ParseVersion("1.14rc2")
	v4 := ParseVersion("1.13rc3")
	versions := Versions{v0, v1, v2, v3, v4}
	sort.Sort(versions)
	require.Equal(t, Versions{v4, v3, v0, v2, v1}, versions)
}

func TestSortReferences(t *testing.T) {
	v0 := ParseReference("go-1.13")
	v1 := ParseReference("go-1.13.5")
	v2 := ParseReference("go-1.13.4")
	v3 := ParseReference("go-1.14rc2")
	v4 := ParseReference("go-1.13rc3")
	v5 := ParseReference("go@stable")
	refs := References{v0, v1, v2, v3, v4, v5}
	sort.Sort(refs)
	require.Equal(t, References{v5, v4, v3, v0, v2, v1}, refs)
}
