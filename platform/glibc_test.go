package platform

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestParseGlibcVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    GlibcVersion
		wantErr string
	}{
		{input: "2.28", want: GlibcVersion{Major: 2, Minor: 28}},
		{input: "2.3", want: GlibcVersion{Major: 2, Minor: 3}},
		{input: "2", wantErr: `invalid glibc version "2", expected <major>.<minor>`},
		{input: "2.28.1", wantErr: `invalid glibc version "2.28.1", expected <major>.<minor>`},
		{input: "2.x", wantErr: `invalid glibc version "2.x", expected <major>.<minor>`},
		{input: "-2.28", wantErr: `invalid glibc version "-2.28", expected <major>.<minor>`},
		{input: "", wantErr: `invalid glibc version "", expected <major>.<minor>`},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := ParseGlibcVersion(test.input)
			if test.wantErr != "" {
				assert.EqualError(t, err, test.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestGlibcVersionCompare(t *testing.T) {
	v := GlibcVersion{Major: 2, Minor: 28}
	assert.Equal(t, 0, v.Compare(GlibcVersion{Major: 2, Minor: 28}))
	assert.Equal(t, 1, v.Compare(GlibcVersion{Major: 2, Minor: 9}))
	assert.Equal(t, -1, v.Compare(GlibcVersion{Major: 2, Minor: 35}))
	assert.Equal(t, -1, v.Compare(GlibcVersion{Major: 3, Minor: 0}))
}

func TestParseGetconfGlibcVersion(t *testing.T) {
	tests := []struct {
		output string
		want   GlibcVersion
	}{
		{output: "glibc 2.35\n", want: GlibcVersion{Major: 2, Minor: 35}},
		{output: "glibc 2.39.9000\n", want: GlibcVersion{Major: 2, Minor: 39}},
		{output: "musl 1.2.4\n"},
		{output: "glibc\n"},
		{output: ""},
	}
	for _, test := range tests {
		t.Run(test.output, func(t *testing.T) {
			assert.Equal(t, test.want, parseGetconfGlibcVersion(test.output))
		})
	}
}

func TestPlatformString(t *testing.T) {
	assert.Equal(t, "linux-amd64", Platform{OS: Linux, Arch: Amd64}.String())
	assert.Equal(t, "linux-amd64-glibc2.28", Platform{OS: Linux, Arch: Amd64, Glibc: GlibcVersion{Major: 2, Minor: 28}}.String())
}
