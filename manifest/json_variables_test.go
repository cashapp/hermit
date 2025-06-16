package manifest

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/platform"
)

type testJSONHTTPClient struct {
	jsonData string
}

func (t testJSONHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(t.jsonData)),
	}, nil
}

func TestJSONVariableInjection(t *testing.T) {
	jsonData := `{
		"version": "1.2.3",
		"build_number": "20250117151628",
		"checksum": "abc123def456"
	}`

	client := &http.Client{
		Transport: testJSONHTTPClient{jsonData: jsonData},
	}

	manifest := &AnnotatedManifest{
		Name: "test-package",
		Manifest: &Manifest{
			Description: "Test package for JSON variables",
			Versions: []VersionBlock{
				{
					Version: []string{"1.0.0"},
					AutoVersion: &AutoVersionBlock{
						JSON: &JSONAutoVersionBlock{
							URL:  "https://api.example.com/version.json",
							Path: "version",
						},
						VersionPattern: "(.*)",
					},
					Layer: Layer{
						Source: "https://example.com/package-${version}-${json:build_number}.tar.gz",
						Binaries: []string{"test-binary"},
						Vars: map[string]string{
							"build_number": "${json:build_number}",
						},
						SHA256: "${json:checksum}",
					},
				},
			},
		},
	}

	config := Config{
		Env:        "/tmp/hermit-test",
		State:      "/tmp/hermit-state",
		HTTPClient: client,
		Platform:   platform.Platform{OS: "linux", Arch: "amd64"},
	}

	ref := Reference{Name: "test-package", Version: ParseVersion("1.0.0")}
	pkg, err := Resolve(manifest, config, ref)

	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/package-1.0.0-20250117151628.tar.gz", pkg.Source)
	assert.Equal(t, "20250117151628", pkg.Vars["build_number"])
	assert.Equal(t, "abc123def456", pkg.SHA256)
}