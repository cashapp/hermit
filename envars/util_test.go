package envars

import (
	"strconv"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/platform"
)

func TestExpandMapping(t *testing.T) {
	cases := []struct {
		name       string
		mapping    func(string) string
		wantExpand map[string]string
	}{
		{
			name: "basic expansion",
			mapping: Mapping("hermit/env", "home/user", platform.Platform{
				OS:   platform.Linux,
				Arch: platform.Amd64,
			}),
			wantExpand: map[string]string{
				"${HOME}":       "home/user",
				"${HERMIT_ENV}": "hermit/env",
				"${env}":        "hermit/env",
				"${HERMIT_BIN}": "hermit/env/bin",
				"${os}":         platform.Linux,
				"${arch}":       platform.Amd64,
				"${xarch}":      platform.ArchToXArch(platform.Amd64),
				"${NOT_A_VAR}":  "",
			},
		},
		{
			name: "nested expansion",
			mapping: Mapping("${HOME}/env", "home/${os}-user", platform.Platform{
				OS:   platform.Darwin,
				Arch: platform.Arm64,
			}),
			wantExpand: map[string]string{
				"You live at $HOME!":         "You live at home/darwin-user!",
				"${HERMIT_ENV}":              "home/darwin-user/env",
				"${env}":                     "home/darwin-user/env",
				"${HERMIT_BIN}":              "home/darwin-user/env/bin",
				"$HERMIT_BIN/foo-$arch/$env": "home/darwin-user/env/bin/foo-arm64/home/darwin-user/env",
				"$xarch":                     platform.ArchToXArch(platform.Arm64),
			},
		},
		{
			name: "unknown OS and Arch",
			mapping: Mapping("", "", platform.Platform{
				OS:   "foo",
				Arch: "bar",
			}),
			wantExpand: map[string]string{
				"${os}-${arch}-${xarch}": "foo-bar-bar",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for str, wantExpand := range tc.wantExpand {
				assert.Equal(t, wantExpand, Expand(str, tc.mapping))
			}
		})
	}
}

func TestExpandDateTime(t *testing.T) {
	mapping := Mapping("foo", "foo", platform.Platform{})

	// Testing time expansion is complicated, so we simply make sure
	// that year, month, and day are each expanded to positive integers.
	for _, pattern := range []string{"$DD", "$MM", "$YYYY"} {
		gotExpand := Expand(pattern, mapping)
		gotInt, err := strconv.Atoi(gotExpand)
		assert.NoError(t, err, "could not convert datetime expansion of %v (%v) to int", pattern, gotExpand)
		assert.NotZero(t, gotInt, "expected nonzero date")
	}
}
