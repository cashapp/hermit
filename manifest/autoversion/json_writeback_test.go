package autoversion

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/github"
)

// TestJSONAutoVersionWriteback tests that variables are written back using extract blocks
func TestJSONAutoVersionWriteback(t *testing.T) {
	// Create a test manifest with new extract block format
	manifestContent := `description = "Test package"
binaries = ["test"]

version "1.0.0" {
  source = "https://example.com/test-${version}.tar.gz"
  
  auto-version {
    json {
      url = "https://api.example.com/releases/latest.json"
      
      extract {
        version = "tag_name"
        
        platform {
          build_number = "build.number"
          commit_sha = "commit.sha"
          sha256 = "assets.0.sha256"
        }
      }
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

	// Read the updated manifest and compare to expected output
	updatedContent, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)

	// Verify the new behavior: single version block with multiple labels and top-level vars cache
	actualContent := string(updatedContent)

	// Should have a vars block inside auto-version with version-specific resolved variables
	assert.Contains(t, actualContent, "auto-version {", "Should have auto-version block")
	assert.Contains(t, actualContent, "vars {", "Should have vars block within auto-version")
	assert.Contains(t, actualContent, `1.2.3 {`, "Should have version block within vars")
	assert.Contains(t, actualContent, `build_number = "20250117151628"`, "Should have build number in vars cache")
	assert.Contains(t, actualContent, `commit_sha = "abc123def456"`, "Should have commit SHA in vars cache")
	assert.Contains(t, actualContent, `sha256 = "a1b2c3d4e5f6"`, "Should have SHA256 in vars cache")

	// Should have single version block with multiple labels
	assert.Contains(t, actualContent, `version "1.0.0" "1.2.3"`, "Version block should have multiple labels")

	// Should only have one version block
	versionCount := strings.Count(actualContent, "version \"")
	assert.Equal(t, 1, versionCount, "Should have exactly 1 version block")
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
