package sources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
)

func TestEnvSourceRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	env := filepath.Join(root, "env")
	outside := filepath.Join(root, "outside")
	assert.NoError(t, os.MkdirAll(env, 0750))
	assert.NoError(t, os.MkdirAll(outside, 0750))

	for _, uri := range []redact.URL{
		"env:///../outside",
		"env:///%2e%2e/outside",
	} {
		t.Run(uri.String(), func(t *testing.T) {
			ui, _ := ui.NewForTesting()
			_, err := ForURIs(ui, filepath.Join(root, "state"), env, []redact.URL{uri})
			assert.Error(t, err)
		})
	}
}

func TestGitHubTokenRewriter(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		token   string
		pattern string
		want    string
	}{
		{
			name:    "matching github repo",
			uri:     "https://github.com/owner/repo.git",
			token:   "secret-token",
			pattern: "owner/*",
			want:    "https://x-access-token:secret-token@github.com/owner/repo.git",
		},
		{
			name:    "non-matching github repo",
			uri:     "https://github.com/other/repo.git",
			token:   "secret-token",
			pattern: "owner/*",
			want:    "https://github.com/other/repo.git",
		},
		{
			name:    "non-github url",
			uri:     "https://example.com/repo.git",
			token:   "secret-token",
			pattern: "*/*",
			want:    "https://example.com/repo.git",
		},
		{
			name:    "git protocol url",
			uri:     "git@github.com:owner/repo.git",
			token:   "secret-token",
			pattern: "owner/*",
			want:    "git@github.com:owner/repo.git",
		},
		{
			name:    "git protocol url with matching pattern",
			uri:     "git@github.com:owner/repo.git",
			token:   "secret-token",
			pattern: "*/*",
			want:    "git@github.com:owner/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := github.GlobRepoMatcher([]string{tt.pattern})
			assert.NoError(t, err)

			rewriter := github.AuthenticatedURLRewriter(redact.Secret(tt.token), matcher)
			result, err := rewriter(redact.URL(tt.uri))

			assert.NoError(t, err)
			assert.Equal(t, tt.want, result.Reveal())
			assert.Equal(t, tt.uri, result.String())
		})
	}
}

// TestForURIsIntegration tests the integration of ForURIs with rewriters
func TestForURIsIntegration(t *testing.T) {
	l, _ := ui.NewForTesting()

	t.Run("successful rewriting", func(t *testing.T) {
		matcher, err := github.GlobRepoMatcher([]string{"owner/*"})
		assert.NoError(t, err)
		rewriter := github.AuthenticatedURLRewriter(redact.Secret("test-token"), matcher)

		uris := []redact.URL{
			"https://github.com/owner/repo1.git",
			"https://github.com/other/repo2.git",
			"git@github.com:owner/repo3.git",
		}
		sources, err := ForURIs(l, "testdir", "testenv", uris, rewriter)

		assert.NoError(t, err)
		assert.Equal(t, len(uris), len(sources.sources))

		gitSource, ok := sources.sources[0].(*GitSource)
		assert.True(t, ok)
		assert.Equal(t, "https://x-access-token:test-token@github.com/owner/repo1.git", gitSource.remote.Reveal())
		assert.Equal(t, "https://github.com/owner/repo1.git", sources.sources[0].URI())
		assert.Equal(t, uris[1].Reveal(), sources.sources[1].URI())
		assert.Equal(t, uris[2].Reveal(), sources.sources[2].URI())
	})

	t.Run("rewriter error", func(t *testing.T) {
		errorRewriter := func(uri redact.URL) (redact.URL, error) {
			return "", errors.New("rewriter error")
		}

		uris := []redact.URL{"https://github.com/owner/repo.git"}
		_, err := ForURIs(l, "testdir", "testenv", uris, errorRewriter)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rewriter error")
	})

	t.Run("git remote helper uri", func(t *testing.T) {
		_, err := ForURIs(l, "testdir", "testenv", []redact.URL{"zzq::x.git"})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remote helpers are not supported")
	})

	t.Run("invalid rewritten uri", func(t *testing.T) {
		invalidRewriter := func(uri redact.URL) (redact.URL, error) {
			return "invalid://not-a-valid-source", nil
		}

		uris := []redact.URL{"https://github.com/owner/repo.git"}
		_, err := ForURIs(l, "testdir", "testenv", uris, invalidRewriter)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported source")
	})
}
