package autoversion

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit/github"
)

// TestJSONAutoVersionWriteback tests that variables and SHA256 are written back to the manifest
func TestJSONAutoVersionWriteback(t *testing.T) {
	// Create a test manifest with JSON auto-version
	manifestContent := `description = "Test package"
binaries = ["test"]

version "1.0.0" {
  source = "https://example.com/test-${version}.tar.gz"
  
  auto-version {
    json {
      url = "https://api.example.com/releases/latest.json"
      path = "tag_name"
      sha256-path = "assets.0.sha256"
    }
    version-pattern = "v(.*)"
  }
}
`

	// Create a test HTTP client that returns our test JSON
	testJSON := `{
  "tag_name": "v1.2.3",
  "build": {
    "number": "20250117151628"
  },
  "commit": {
    "sha": "abc123def456"
  },
  "assets": [
    {
      "name": "test-1.2.3.tar.gz",
      "sha256": "a1b2c3d4e5f6"
    }
  ]
}`

	client := &http.Client{
		Transport: &testRoundTripper{response: testJSON},
	}

	// Create a temporary file for testing
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "test.hcl")
	err := os.WriteFile(manifestPath, []byte(manifestContent), 0600)
	assert.NoError(t, err)

	// Run auto-version
	latestVersion, err := AutoVersion(client, &mockGitHubClient{}, manifestPath)
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3", latestVersion)

	// Read the updated manifest
	updatedContent, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)

	// Parse the updated manifest to verify the changes
	ast, err := hcl.ParseBytes(updatedContent)
	assert.NoError(t, err)

	// Find the version block (should now contain both 1.0.0 and 1.2.3)
	var versionBlock *hcl.Block
	err = hcl.Visit(ast, func(node hcl.Node, next func() error) error {
		if block, ok := node.(*hcl.Block); ok && block.Name == "version" {
			// The block should contain the new version 1.2.3
			for _, label := range block.Labels {
				if label == "1.2.3" {
					versionBlock = block
					break
				}
			}
		}
		return next()
	})
	assert.NoError(t, err)
	assert.True(t, versionBlock != nil, "Should find the version block with the new version")

	// Check that sha256 was written back
	var sha256Found bool

	for _, entry := range versionBlock.Body {
		if entry.Attribute != nil && entry.Attribute.Key == "sha256" {
			sha256Found = true
			assert.True(t, entry.Attribute.Value.Str != nil)
			assert.Equal(t, "a1b2c3d4e5f6", *entry.Attribute.Value.Str)
		}
	}

	assert.True(t, sha256Found, "sha256 should be written back to the manifest")
}

// testRoundTripper is a simple HTTP transport that returns a fixed response
type testRoundTripper struct {
	response string
}

func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       &nopCloser{strings.NewReader(t.response)},
		Header:     make(http.Header),
	}, nil
}

// nopCloser wraps a reader to add a no-op Close method
type nopCloser struct {
	*strings.Reader
}

func (n *nopCloser) Close() error {
	return nil
}

// mockGitHubClient is a mock implementation of GitHubClient
type mockGitHubClient struct{}

func (m *mockGitHubClient) LatestRelease(repo string) (*github.Release, error) {
	return nil, nil
}

func (m *mockGitHubClient) Releases(repo string, limit int) ([]*github.Release, error) {
	return nil, nil
}
