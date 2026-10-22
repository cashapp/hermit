package platform

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestPlatformMatch(t *testing.T) {
	linux := Platform{OS: Linux, Arch: Amd64, Glibc: GlibcVersion{Major: 2, Minor: 28}}
	tests := []struct {
		name     string
		platform Platform
		label    string
		want     bool
		wantErr  string
	}{
		{name: "OS", platform: linux, label: "linux", want: true},
		{name: "Arch", platform: linux, label: "amd64", want: true},
		{name: "OSArch", platform: linux, label: "(darwin|linux)-amd64", want: true},
		{name: "OtherOS", platform: linux, label: "darwin"},
		{name: "GlibcOlder", platform: linux, label: "glibc_2_17", want: true},
		{name: "GlibcSame", platform: linux, label: "glibc_2_28", want: true},
		{name: "GlibcNewer", platform: linux, label: "glibc_2_35"},
		{name: "NoGlibc", platform: Platform{OS: Linux, Arch: Amd64}, label: "glibc_2_17"},
		{name: "InvalidGlibc", platform: linux, label: "glibc>=2.28", wantErr: `invalid platform label "glibc>=2.28", expected glibc_<major>_<minor>`},
		{name: "InvalidGlibcVersion", platform: linux, label: "glibc_2_x", wantErr: `invalid platform label "glibc_2_x", expected glibc_<major>_<minor>`},
		{name: "InvalidRegex", platform: linux, label: "(", wantErr: "invalid platform label \"(\": error parsing regexp: missing closing ): `(`"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.platform.Match(test.label)
			if test.wantErr != "" {
				assert.EqualError(t, err, test.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestPlatformVariants(t *testing.T) {
	labels := []string{"linux", "glibc_2_28", "amd64", "glibc_2_17", "glibc_2_28"}
	linux, err := Platform{OS: Linux, Arch: Amd64}.Variants(labels)
	assert.NoError(t, err)
	assert.Equal(t, Platforms{
		{OS: Linux, Arch: Amd64},
		{OS: Linux, Arch: Amd64, Glibc: GlibcVersion{Major: 2, Minor: 17}},
		{OS: Linux, Arch: Amd64, Glibc: GlibcVersion{Major: 2, Minor: 28}},
	}, linux)

	darwin, err := Platform{OS: Darwin, Arch: Arm64}.Variants(labels)
	assert.NoError(t, err)
	assert.Equal(t, Platforms{{OS: Darwin, Arch: Arm64}}, darwin)

	_, err = Platform{OS: Darwin, Arch: Arm64}.Variants([]string{"glibc_2"})
	assert.EqualError(t, err, `invalid platform label "glibc_2", expected glibc_<major>_<minor>`)
}
