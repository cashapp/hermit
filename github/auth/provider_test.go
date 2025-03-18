package auth

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/ui"
)

func TestEnvProvider(t *testing.T) {
	ui, _ := ui.NewForTesting()
	provider := &EnvProvider{ui: ui}

	t.Run("no tokens set", func(t *testing.T) {
		os.Unsetenv("HERMIT_GITHUB_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
		token, err := provider.GetToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
	})

	t.Run("HERMIT_GITHUB_TOKEN set", func(t *testing.T) {
		t.Setenv("HERMIT_GITHUB_TOKEN", "hermit-token")
		t.Setenv("GITHUB_TOKEN", "")
		token, err := provider.GetToken()
		assert.NoError(t, err)
		assert.Equal(t, "hermit-token", token)
	})

	t.Run("GITHUB_TOKEN set", func(t *testing.T) {
		t.Setenv("HERMIT_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "github-token")
		token, err := provider.GetToken()
		assert.NoError(t, err)
		assert.Equal(t, "github-token", token)
	})

	t.Run("both tokens set, HERMIT_GITHUB_TOKEN takes precedence", func(t *testing.T) {
		t.Setenv("HERMIT_GITHUB_TOKEN", "hermit-token")
		t.Setenv("GITHUB_TOKEN", "github-token")
		token, err := provider.GetToken()
		assert.NoError(t, err)
		assert.Equal(t, "hermit-token", token)
	})
}

func TestGHCliProvider(t *testing.T) {
	ui, _ := ui.NewForTesting()
	provider := &GHCliProvider{ui: ui}

	t.Run("gh not installed", func(t *testing.T) {
		t.Setenv("PATH", "")

		token, err := provider.GetToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.Contains(t, err.Error(), "gh CLI not found")
	})

	t.Run("token caching", func(t *testing.T) {
		// Skip if gh not installed
		if _, err := exec.LookPath("gh"); err != nil {
			t.Skip("gh CLI not installed")
		}

		// First call should get a real token
		token1, err := provider.GetToken()
		if err != nil {
			t.Skip("gh auth token failed, probably not authenticated")
		}
		assert.NotEqual(t, "", token1)

		// Second call should return cached token
		token2, err := provider.GetToken()
		assert.NoError(t, err)
		assert.Equal(t, token1, token2)
	})
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		wantType    Provider
		wantErrText string
	}{
		{
			name:     "env provider",
			provider: "env",
			wantType: &EnvProvider{},
		},
		{
			name:     "empty string defaults to env",
			provider: "",
			wantType: &EnvProvider{},
		},
		{
			name:     "gh-cli provider",
			provider: "gh-cli",
			wantType: &GHCliProvider{},
		},
		{
			name:        "unknown provider",
			provider:    "unknown",
			wantErrText: "unknown GitHub token provider: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui, _ := ui.NewForTesting()
			got, err := NewProvider(tt.provider, ui)
			if tt.wantErrText != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrText)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("%T", tt.wantType), fmt.Sprintf("%T", got))
		})
	}
}
