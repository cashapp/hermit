package sources_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
)

func TestEnvSourceRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	env := filepath.Join(root, "env")
	outside := filepath.Join(root, "outside")
	assert.NoError(t, os.MkdirAll(env, 0750))
	assert.NoError(t, os.MkdirAll(outside, 0750))

	for _, uri := range []string{
		"env:///../outside",
		"env:///%2e%2e/outside",
	} {
		t.Run(uri, func(t *testing.T) {
			ui, _ := ui.NewForTesting()
			_, err := sources.ForURIs(ui, filepath.Join(root, "state"), env, []string{uri})
			assert.Error(t, err)
		})
	}
}

func TestInvalidSourceDoesNotLeakCredentials(t *testing.T) {
	l, _ := ui.NewForTesting()
	_, err := sources.ForURIs(l, "testdir", "testenv", []string{
		"https://x-access-token:secret-token@github.com/owner/%zz",
	})

	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-token")
	assert.Contains(t, err.Error(), "https://x-access-token:****@github.com/owner/%zz")
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

			rewriter := github.AuthenticatedURLRewriter(tt.token, matcher)
			result, err := rewriter(sources.NewSourceURI(tt.uri))

			assert.NoError(t, err)
			assert.Equal(t, tt.want, result.Get())
		})
	}
}

func TestGitHubTokenRewriterErrorDoesNotLeakCredentials(t *testing.T) {
	matcher, err := github.GlobRepoMatcher([]string{"*/*"})
	assert.NoError(t, err)
	rewriter := github.AuthenticatedURLRewriter("unused", matcher)

	_, err = rewriter(sources.NewSourceURI("https://x-access-token:secret-token@github.com/owner/%zz"))
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-token")
	assert.Contains(t, err.Error(), "https://x-access-token:****@github.com/owner/%zz")
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
		sourceSet, err := sources.ForURIs(l, "testdir", "testenv", uris, rewriter)

		assert.NoError(t, err)
		assert.Equal(t, len(uris), len(sourceSet.Sources()))

		// Verify the sources were created with appropriate URIs
		// First URI should be rewritten with token, others should remain unchanged
		assert.Contains(t, sourceSet.Sources()[0].Get(), "x-access-token:test-token@github.com")
		assert.Equal(t, "https://x-access-token:****@github.com/owner/repo1.git", sourceSet.Sources()[0].String())
		assert.Equal(t, uris[1], sourceSet.Sources()[1].Get())
		assert.Equal(t, uris[2], sourceSet.Sources()[2].Get())
	})

	t.Run("rewriter error", func(t *testing.T) {
		errorRewriter := func(_ sources.SourceURI) (sources.SourceURI, error) {
			return sources.SourceURI{}, errors.New("rewriter error")
		}

		uris := []string{"https://github.com/owner/repo.git"}
		_, err := sources.ForURIs(l, "testdir", "testenv", uris, errorRewriter)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rewriter error")
	})

	t.Run("git remote helper uri", func(t *testing.T) {
		_, err := sources.ForURIs(l, "testdir", "testenv", []string{"zzq::x.git"})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remote helpers are not supported")
	})

	t.Run("invalid rewritten uri", func(t *testing.T) {
		invalidRewriter := func(_ sources.SourceURI) (sources.SourceURI, error) {
			return sources.NewSourceURI("invalid://not-a-valid-source"), nil
		}

		uris := []string{"https://github.com/owner/repo.git"}
		_, err := sources.ForURIs(l, "testdir", "testenv", uris, invalidRewriter)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported source")
	})
}
