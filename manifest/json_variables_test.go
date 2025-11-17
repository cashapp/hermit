package manifest

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/platform"
)

// TestJSONVariableInjection tests the new pre-population approach for JSON variables.
// This test validates that the Vars field is properly populated on the Package struct.
// JSON auto-version now pre-populates variables during the auto-versioning phase.
func TestJSONVariableInjection(t *testing.T) {
	manifest := &AnnotatedManifest{
		Name: "test-package",
		Manifest: &Manifest{
			Description: "Test package for JSON variables",
			Versions: []VersionBlock{
				{
					Version: []string{"1.0.0"},
					Layer: Layer{
						Source:   "https://example.com/test-${version}.tar.gz",
						SHA256:   "abc123def456",
						Binaries: []string{"test"},
						Vars: map[string]string{
							"path":         "foo/bar",
							"build_number": "20250117151628", // Pre-populated by auto-version
						},
					},
				},
			},
		},
	}

	config := Config{
		Env:      "/tmp/hermit-test",
		State:    "/tmp/hermit-state",
		Platform: platform.Platform{OS: "linux", Arch: "amd64"},
	}

	ref := Reference{Name: "test-package", Version: ParseVersion("1.0.0")}
	pkg, err := Resolve(manifest, config, ref)
	assert.NoError(t, err)

	// Check that the variables are properly populated including pre-populated ones
	expectedVars := map[string]string{
		"path":         "foo/bar",
		"build_number": "20250117151628",
	}
	assert.Equal(t, expectedVars, pkg.Vars)
}
