package autoversion

import (
	"testing"

	"github.com/cashapp/hermit/manifest"
	"github.com/stretchr/testify/require"
)

func TestGitHubVersions(t *testing.T) {
	tests := []struct {
		name            string
		ignoreInvalid   bool
		versions        []string
		versionPattern  string
		expectedVersion string
		error           bool
	}{
		{
			name:            "valid version",
			versions:        []string{"v3.2"},
			versionPattern:  "v(.*)",
			expectedVersion: "3.2",
		},
		{
			name:           "invalid version",
			versions:       []string{"3.2"},
			versionPattern: "v(.*)",
			error:          true,
		},
		{
			name:            "ignore invalid version with no valid versions",
			versions:        []string{"kyaml/v0.13.9", "api/v3.2.2"},
			versionPattern:  "kustomize/v(.*)",
			ignoreInvalid:   true,
			expectedVersion: "",
			error:           true,
		},
		{
			name:            "ignore invalid version containing valid version",
			versions:        []string{"kyaml/v0.13.9", "kustomize/v3.2.2"},
			versionPattern:  "kustomize/v(.*)",
			ignoreInvalid:   true,
			expectedVersion: "3.2.2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, err := gitHub(testGHAPI(test.versions), &manifest.AutoVersionBlock{
				VersionPattern:        test.versionPattern,
				IgnoreInvalidVersions: test.ignoreInvalid,
			})
			if test.error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.expectedVersion, version)
		})
	}
}
