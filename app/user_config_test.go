package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestLoadUserConfig(t *testing.T) {
	// Create a temporary directory for our test files
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		configContents string
		expected       UserConfig
		expectError    bool
	}{
		{
			name: "basic config with init-sources",
			configContents: `
init-sources = [
	"source1",
	"source2"
]`,
			expected: UserConfig{
				Prompt:      "env", // Default value
				InitSources: []string{"source1", "source2"},
			},
		},
		{
			name: "full config",
			configContents: `
prompt = "short"
short-prompt = true
no-git = true
idea = true
init-sources = [
	"source1",
	"source2"
]`,
			expected: UserConfig{
				Prompt:      "short",
				ShortPrompt: true,
				NoGit:       true,
				Idea:        true,
				InitSources: []string{"source1", "source2"},
			},
		},
		{
			name:           "empty config",
			configContents: "",
			expected: UserConfig{
				Prompt: "env", // Default value
			},
		},
		{
			name: "invalid HCL",
			configContents: `
init-sources = [
	"unclosed array
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary config file
			configPath := filepath.Join(tmpDir, "config.hcl")
			err := os.WriteFile(configPath, []byte(tt.configContents), 0644)
			assert.NoError(t, err)

			// Load the config
			config, err := LoadUserConfig(configPath)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected.InitSources, config.InitSources)
			assert.Equal(t, tt.expected.Prompt, config.Prompt)
			assert.Equal(t, tt.expected.ShortPrompt, config.ShortPrompt)
			assert.Equal(t, tt.expected.NoGit, config.NoGit)
			assert.Equal(t, tt.expected.Idea, config.Idea)
		})
	}
}
