package manifest

import (
	"sort"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestReferenceMatch(t *testing.T) {
	assert.True(t, ParseReference("go-1.13").Match(ParseReference("go-1.13.7")))
	assert.False(t, ParseReference("go-1.12").Match(ParseReference("go-1.13.7")))
	assert.False(t, ParseReference("protoc-3.15.0-square-1.0").Match(ParseReference("protoc-3.15.0")))
	assert.False(t, ParseReference("protoc-3.15.0-square-1.0").Match(ParseReference("protoc-3.15.0-square-1")))
	assert.True(t, ParseReference("protoc-3.15.0-square-1").Match(ParseReference("protoc-3.15.0-square-1.0")))
	assert.True(t, ParseReference("protoc-3.15.0-square-1.0").Match(ParseReference("protoc-3.15.0-square-1.0")))
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
			assert.Equal(t, test.parts, strings.Join(v.Components(), "."))
			assert.Equal(t, test.prerelease, v.Prerelease())
			assert.Equal(t, test.metadata, v.Metadata())
		})
	}
}

func TestParseVersions(t *testing.T) {
	tests := []struct {
		version    string
		clean      string
		majorMinor string
		major      string
		parts      string
		prerelease string
		metadata   string
	}{
		{version: "1.2.3", clean: "1.2.3", majorMinor: "1.2",
			major: "1", parts: "1.2.3"},
		{version: "1.5.1-kotlin.3", clean: "1.5.1", majorMinor: "1.5-kotlin.3",
			major: "1-kotlin.3", parts: "1.5.1", prerelease: "kotlin.3"},
		{version: "11.0.10_9", clean: "11.0.10_9", majorMinor: "11.0",
			major: "11", parts: "11.0.10.9"},
		{version: "1.2.3+meta", clean: "1.2.3", majorMinor: "1.2+meta",
			major: "1+meta", parts: "1.2.3", metadata: "meta"},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			v := ParseVersion(test.version)
			assert.Equal(t, test.version, v.String())
			assert.Equal(t, test.clean, v.Clean().String())
			assert.Equal(t, test.majorMinor, v.MajorMinor().String())
			assert.Equal(t, test.major, v.Major().String())
			assert.Equal(t, test.parts, strings.Join(v.Components(), "."))
			assert.Equal(t, test.prerelease, v.Prerelease())
			assert.Equal(t, test.metadata, v.Metadata())
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
	assert.Equal(t, Versions{v4, v3, v0, v2, v1}, versions)
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
	assert.Equal(t, References{v5, v4, v3, v0, v2, v1}, refs)
}
