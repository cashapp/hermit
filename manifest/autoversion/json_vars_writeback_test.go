package autoversion

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/hcl"
)

// TestJSONAutoVersionVarsWriteback tests that variables are written back to the manifest
func TestJSONAutoVersionVarsWriteback(t *testing.T) {
	// Create a test manifest with JSON auto-version that extracts variables
	manifestContent := `description = "Test package"
binaries = ["test"]

version "1.0.0" {
  source = "https://example.com/test-${version}.tar.gz"
  
  auto-version {
    json {
      url = "https://api.example.com/releases/latest.json"
      path = "tag_name"
      vars = {
        "build_number": "build.number",
        "commit_sha": "commit.sha"
      }
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

	// Check that vars and sha256 were written back
	var varsFound bool
	var sha256Found bool

	for _, entry := range versionBlock.Body {
		if entry.Attribute != nil {
			switch entry.Attribute.Key {
			case "vars":
				varsFound = true
				assert.True(t, entry.Attribute.Value.HaveMap, "vars should be a map")

				// Check that the expected variables are present
				varMap := make(map[string]string)
				for _, mapEntry := range entry.Attribute.Value.Map {
					if mapEntry.Key.Str != nil && mapEntry.Value.Str != nil {
						varMap[*mapEntry.Key.Str] = *mapEntry.Value.Str
					}
				}
				assert.Equal(t, "20250117151628", varMap["build_number"])
				assert.Equal(t, "abc123def456", varMap["commit_sha"])

			case "sha256":
				sha256Found = true
				assert.True(t, entry.Attribute.Value.Str != nil)
				assert.Equal(t, "a1b2c3d4e5f6", *entry.Attribute.Value.Str)
			}
		}
	}

	assert.True(t, varsFound, "vars should be written back to the manifest")
	assert.True(t, sha256Found, "sha256 should be written back to the manifest")
}
