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
		latestVersion   string
		expectedVersion string
		error           bool
	}{
		{
			name:            "valid version",
			latestVersion:   "v3.2",
			expectedVersion: "3.2",
		},
		{
			name:          "invalid version",
			latestVersion: "3.2",
			error:         true,
		},
		{
			name:            "ignore invalid version",
			latestVersion:   "3.2",
			ignoreInvalid:   true,
			expectedVersion: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, err := gitHub(testGHAPI(test.latestVersion), &manifest.AutoVersionBlock{
				VersionPattern:        "v(.*)",
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
