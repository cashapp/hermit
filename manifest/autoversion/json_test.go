package autoversion

import (
	"net/http"
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/manifest"
)

func TestJSONAutoVersion(t *testing.T) {
	tests := []struct {
		name            string
		json            string
		jq              string
		versionPattern  string
		ignoreInvalid   bool
		expectedVersion string
		expectedError   string
	}{
		{
			name:            "ArrayOfObjects",
			json:            `{"releases":[{"tag":"v1.0.0"},{"tag":"v2.0.0"},{"tag":"v1.5.0"}]}`,
			jq:              `.releases[].tag`,
			versionPattern:  `v(.*)`,
			expectedVersion: "2.0.0",
		},
		{
			name:            "SimpleArray",
			json:            `["3.1.0","2.0.0","3.2.1"]`,
			jq:              `.[]`,
			expectedVersion: "3.2.1",
		},
		{
			name:            "IgnoreInvalidVersions",
			json:            `["v1.0.0","latest","v2.0.0"]`,
			jq:              `.[]`,
			versionPattern:  `v(\d+\.\d+\.\d+)`,
			ignoreInvalid:   true,
			expectedVersion: "2.0.0",
		},
		{
			name:           "InvalidVersionNotIgnored",
			json:           `["v1.0.0","latest"]`,
			jq:             `.[]`,
			versionPattern: `v(\d+\.\d+\.\d+)`,
			expectedError:  "version must match the pattern",
		},
		{
			name:          "BadJQExpression",
			json:          `{}`,
			jq:            `.foo bar`,
			expectedError: "could not parse jq expression",
		},
		{
			name:          "NoMatches",
			json:          `{"releases":[]}`,
			jq:            `.releases[].tag`,
			expectedError: "no versions matched",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := t.TempDir() + "/response.json"
			err := writeTestFile(tmpFile, tt.json)
			assert.NoError(t, err)

			versionPattern := tt.versionPattern
			if versionPattern == "" {
				versionPattern = "v?(.*)"
			}

			version, err := jsonAutoVersion(&http.Client{
				Transport: testHTTPClient{path: tmpFile},
			}, &manifest.AutoVersionBlock{
				JSON: &manifest.JSONAutoVersionBlock{
					URL: "http://example.com/api",
					JQ:  tt.jq,
				},
				VersionPattern:        versionPattern,
				IgnoreInvalidVersions: tt.ignoreInvalid,
			})

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVersion, version)
			}
		})
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
