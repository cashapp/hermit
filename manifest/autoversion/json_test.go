package autoversion

import (
	"net/http"
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
)

func TestJSONAutoVersion(t *testing.T) {
	tests := []struct {
		name     string
		jsonFile string
		path     string
		pattern  string
		expected string
	}{
		{
			name:     "Simple version extraction",
			jsonFile: "simple.json",
			path:     "version",
			pattern:  "(.*)",
			expected: "1.2.3",
		},
		{
			name:     "Array version extraction",
			jsonFile: "array.json",
			path:     "versions",
			pattern:  "v?(.*)",
			expected: "2.1.0",
		},
		{
			name:     "Complex path extraction",
			jsonFile: "complex.json",
			path:     "releases.latest.tag_name",
			pattern:  "v(.*)",
			expected: "1.5.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := jsonAutoVersion(&http.Client{
				Transport: testHTTPClient{
					path: "testdata/" + tt.jsonFile,
				},
			}, &manifest.AutoVersionBlock{
				JSON: &manifest.JSONAutoVersionBlock{
					URL:  "http://example.com/" + tt.jsonFile,
					Path: tt.path,
				},
				VersionPattern: tt.pattern,
			})
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, version)
		})
	}
}

func TestJSONAutoVersionErrors(t *testing.T) {
	tests := []struct {
		name        string
		jsonFile    string
		path        string
		pattern     string
		expectedErr string
	}{
		{
			name:        "Invalid JSON",
			jsonFile:    "invalid.json",
			path:        "version",
			pattern:     "(.*)",
			expectedErr: "invalid JSON response",
		},
		{
			name:        "Path not found",
			jsonFile:    "simple.json",
			path:        "nonexistent",
			pattern:     "(.*)",
			expectedErr: "matched no results",
		},
		{
			name:        "Version pattern mismatch",
			jsonFile:    "simple.json",
			path:        "version",
			pattern:     "v(.*)",
			expectedErr: "version must match the pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := jsonAutoVersion(&http.Client{
				Transport: testHTTPClient{
					path: "testdata/" + tt.jsonFile,
				},
			}, &manifest.AutoVersionBlock{
				JSON: &manifest.JSONAutoVersionBlock{
					URL:  "http://example.com/" + tt.jsonFile,
					Path: tt.path,
				},
				VersionPattern: tt.pattern,
			})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

type testHTTPClientWithHeaders struct {
	path            string
	expectedHeaders map[string]string
}

func (t testHTTPClientWithHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	// Verify expected headers are present
	for key, expectedValue := range t.expectedHeaders {
		if actualValue := req.Header.Get(key); actualValue != expectedValue {
			return nil, errors.Errorf("expected header %s=%s, got %s", key, expectedValue, actualValue)
		}
	}

	r, err := os.Open(t.path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &http.Response{
		StatusCode: 200,
		Body:       r,
		ContentLength: func() int64 {
			if stat, err := r.Stat(); err == nil {
				return stat.Size()
			}
			return -1
		}(),
	}, nil
}

func TestJSONHeaders(t *testing.T) {
	_, err := jsonAutoVersion(&http.Client{
		Transport: testHTTPClientWithHeaders{
			path: "testdata/simple.json",
			expectedHeaders: map[string]string{
				"Authorization": "Bearer token123",
				"Accept":        "application/json",
			},
		},
	}, &manifest.AutoVersionBlock{
		JSON: &manifest.JSONAutoVersionBlock{
			URL:  "http://example.com/simple.json",
			Path: "version",
			Headers: map[string]string{
				"Authorization": "Bearer token123",
			},
		},
		VersionPattern: "(.*)",
	})
	assert.NoError(t, err)
}

func TestJSONVariableExtraction(t *testing.T) {
	client := &http.Client{
		Transport: testHTTPClient{
			path: "testdata/gke.http",
		},
	}

	result, err := extractFromJSON(client, &manifest.AutoVersionBlock{
		JSON: &manifest.JSONAutoVersionBlock{
			URL:  "http://example.com/gke.http",
			Path: "components.#(id==\"gke-gcloud-auth-plugin-linux-x86_64\").version.version_string",
			Vars: map[string]string{
				"build_number": "components.#(id==\"gke-gcloud-auth-plugin-linux-x86_64\").version.build_number",
			},
			SHA256Path: "components.#(id==\"gke-gcloud-auth-plugin-linux-x86_64\").data.checksum",
		},
		VersionPattern: "(.*)",
	})

	assert.NoError(t, err)
	assert.Equal(t, "0.5.12", result.Version)
	assert.Equal(t, "20250117151628", result.Variables["build_number"])
	assert.Equal(t, "a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", result.SHA256)
}
