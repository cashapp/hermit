package autoversion

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

// TestJSONAutoVersionNestedBlocks tests that platform-specific ${json:...} variables
// in nested blocks (darwin, linux, platform) are properly resolved
func TestJSONAutoVersionNestedBlocks(t *testing.T) {
	// Create a test manifest with extract block (new format)
	manifestContent := `description = "Test package with platform-specific vars"
binaries = ["test"]

version "1.0.0" {
  source = "https://example.com/test-${version}.tar.gz"

  auto-version {
    json {
      url = "https://api.example.com/releases/latest.json"
      
      extract {
        version = "tag_name"
        
        darwin {
          build_number = "builds.darwin.number"
          sha256 = "builds.darwin.checksum"
        }
        
        linux {
          build_number = "builds.linux.number"
          sha256 = "builds.linux.checksum"
        }
      }
    }
    version-pattern = "v(.*)"
  }

  darwin {
    source = "https://example.com/test-darwin-${version}.tar.gz"
    vars {
      build_number = "${auto-version.vars[version][platform].build_number}"
    }
    sha256 = "${auto-version.vars[version][platform].sha256}"
  }

  linux {
    source = "https://example.com/test-linux-${version}.tar.gz"
    vars {
      build_number = "${auto-version.vars[version][platform].build_number}"
    }
    sha256 = "${auto-version.vars[version][platform].sha256}"
  }
}
`

	// Create a test HTTP client that returns comprehensive JSON with platform-specific data
	testJSON := `{
  "tag_name": "v1.2.3",
  "builds": {
    "darwin": {
      "number": "darwin-20250117151628",
      "checksum": "abc123darwin"
    },
    "linux": {
      "number": "linux-20250117151634",
      "checksum": "def456linux"
    }
  }
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

	// Don't do exact string comparison due to non-deterministic map order
	// Instead verify the key components are present

	// Verify correct behavior: Single version block with multiple labels and top-level vars cache
	actualContent := string(updatedContent)

	// Should have a vars block inside auto-version with nested structure
	assert.Contains(t, actualContent, "auto-version {", "Should have auto-version block")
	assert.Contains(t, actualContent, "vars {", "Should have vars block within auto-version")
	assert.Contains(t, actualContent, `1.2.3 {`, "Should have version block within vars")
	assert.Contains(t, actualContent, `darwin {`, "Should have darwin platform block")
	assert.Contains(t, actualContent, `linux {`, "Should have linux platform block")
	assert.Contains(t, actualContent, `build_number = "darwin-20250117151628"`, "Should have Darwin build number")
	assert.Contains(t, actualContent, `build_number = "linux-20250117151634"`, "Should have Linux build number")
	assert.Contains(t, actualContent, `sha256 = "abc123darwin"`, "Should have Darwin SHA256")
	assert.Contains(t, actualContent, `sha256 = "def456linux"`, "Should have Linux SHA256")

	// Version block should have explicit auto-version.vars references
	assert.Contains(t, actualContent, `"${auto-version.vars[version][platform].build_number}"`, "Version block should use explicit auto-version.vars references")
	assert.Contains(t, actualContent, `"${auto-version.vars[version][platform].sha256}"`, "Version block should use explicit auto-version.vars references")

	// Verify we have exactly 1 version block with multiple labels
	versionCount := strings.Count(actualContent, "version \"")
	assert.Equal(t, 1, versionCount, "Should have exactly 1 version block")
	assert.Contains(t, actualContent, `version "1.0.0" "1.2.3"`, "Version block should have multiple labels")

	// Verify the version still has the auto-version block
	assert.Contains(t, actualContent, "auto-version {", "Version should keep auto-version block for future runs")
}
