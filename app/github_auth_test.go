package app

import (
	"errors"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
)

func TestGitHubTokenForHost(t *testing.T) {
	t.Run("token environment variable takes precedence", func(t *testing.T) {
		t.Setenv("TOKEN_ENV", "configured-token")
		token, source, err := githubTokenForHost(hermit.GitHubTokenAuthConfig{Host: "github.com", TokenEnv: "TOKEN_ENV"})
		assert.NoError(t, err)
		assert.Equal(t, "configured-token", token)
		assert.Equal(t, "TOKEN_ENV", source)
	})

	t.Run("github.com environment variables have stable precedence", func(t *testing.T) {
		t.Setenv("HERMIT_GITHUB_TOKEN", "hermit-token")
		t.Setenv("GITHUB_TOKEN", "github-token")
		token, source, err := githubTokenForHost(hermit.GitHubTokenAuthConfig{Host: "github.com"})
		assert.NoError(t, err)
		assert.Equal(t, "hermit-token", token)
		assert.Equal(t, "HERMIT_GITHUB_TOKEN", source)
	})

	t.Run("enterprise host uses gh", func(t *testing.T) {
		previous := githubTokenFromCLI
		defer func() { githubTokenFromCLI = previous }()
		githubTokenFromCLI = func(host string) (redact.Secret, error) {
			assert.Equal(t, "mycompany.ghe.com", host)
			return "gh-token", nil
		}
		token, source, err := githubTokenForHost(hermit.GitHubTokenAuthConfig{Host: "MyCompany.GHE.com"})
		assert.NoError(t, err)
		assert.Equal(t, "gh-token", token)
		assert.Equal(t, "gh auth token", source)
	})

	t.Run("enterprise gh authentication failure is returned", func(t *testing.T) {
		previous := githubTokenFromCLI
		defer func() { githubTokenFromCLI = previous }()
		githubTokenFromCLI = func(string) (redact.Secret, error) { return "", errors.New("not authenticated") }
		_, _, err := githubTokenForHost(hermit.GitHubTokenAuthConfig{Host: "mycompany.ghe.com"})
		assert.Error(t, err)
	})
}

func TestConfiguredGitHubAuthsNormalizesHostAndMatcher(t *testing.T) {
	t.Setenv("ENTERPRISE_TOKEN", "token")
	p, _ := ui.NewForTesting()
	auths, err := configuredGitHubAuths(p, &hermit.EnvInfo{Config: &hermit.Config{GitHubTokenAuth: []hermit.GitHubTokenAuthConfig{{
		Host:     "https://MYCOMPANY.GHE.COM/",
		TokenEnv: "ENTERPRISE_TOKEN",
		Match:    []string{"owner/*"},
	}}}})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(auths))
	assert.Equal(t, github.HostConfig{WebHost: "mycompany.ghe.com", Token: redact.Secret("token")}, auths[0].host)
	assert.True(t, auths[0].matcher("owner", "repo"))
	assert.False(t, auths[0].matcher("other", "repo"))
}
