package sources

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/ui"
)

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

			rewriter := github.AuthenticatedURLRewriter(tt.token, matcher)
			result, err := rewriter(tt.uri)

			assert.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestForURIsIntegration tests the integration of ForURIs with rewriters
func TestForURIsIntegration(t *testing.T) {
	l, _ := ui.NewForTesting()

	t.Run("successful rewriting", func(t *testing.T) {
		matcher, err := github.GlobRepoMatcher([]string{"owner/*"})
		assert.NoError(t, err)
		rewriter := github.AuthenticatedURLRewriter("test-token", matcher)

		uris := []string{
			"https://github.com/owner/repo1.git",
			"https://github.com/other/repo2.git",
			"git@github.com:owner/repo3.git",
		}
		sources, err := ForURIs(l, "testdir", "testenv", uris, rewriter)

		assert.NoError(t, err)
		assert.Equal(t, len(uris), len(sources.sources))

		// Verify the sources were created with appropriate URIs
		// First URI should be rewritten with token, others should remain unchanged
		assert.Contains(t, sources.sources[0].URI(), "x-access-token:test-token@github.com")
		assert.Equal(t, uris[1], sources.sources[1].URI())
		assert.Equal(t, uris[2], sources.sources[2].URI())
	})

	t.Run("rewriter error", func(t *testing.T) {
		errorRewriter := func(uri string) (string, error) {
			return "", errors.New("rewriter error")
		}

		uris := []string{"https://github.com/owner/repo.git"}
		_, err := ForURIs(l, "testdir", "testenv", uris, errorRewriter)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rewriter error")
	})

	t.Run("invalid rewritten uri", func(t *testing.T) {
		invalidRewriter := func(uri string) (string, error) {
			return "invalid://not-a-valid-source", nil
		}

		uris := []string{"https://github.com/owner/repo.git"}
		_, err := ForURIs(l, "testdir", "testenv", uris, invalidRewriter)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported source")
	})
}
