package sources

import (
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/ui"
)

func TestForURIsAttachesCredentialsWithoutRewritingURIs(t *testing.T) {
	l, _ := ui.NewForTesting()

	matcher, err := github.GlobRepoMatcher([]string{"owner/*"})
	assert.NoError(t, err)
	credentials := github.GitCredentialEnv("secret-token", matcher)

	uris := []string{
		"https://github.com/owner/repo1.git",
		"https://github.com/other/repo2.git",
		"git@github.com:owner/repo3.git",
	}
	sources, err := ForURIs(l, "testdir", "testenv", uris, credentials)
	assert.NoError(t, err)
	assert.Equal(t, len(uris), len(sources.sources))

	for i, uri := range uris {
		assert.Equal(t, uri, sources.sources[i].URI(),
			"source URI must be left untouched so the token cannot leak through it")
	}

	matched, ok := sources.sources[0].(*GitSource)
	assert.True(t, ok)
	assert.NotZero(t, matched.env, "matching repo should carry credentials out of band")

	unmatched, ok := sources.sources[1].(*GitSource)
	assert.True(t, ok)
	assert.Zero(t, unmatched.env, "non-matching repo should carry no credentials")
}

func TestForURIsNeverPlacesTokenInSourceURI(t *testing.T) {
	l, _ := ui.NewForTesting()

	matcher, err := github.GlobRepoMatcher([]string{"*/*"})
	assert.NoError(t, err)

	sources, err := ForURIs(l, "testdir", "testenv",
		[]string{"https://github.com/owner/repo.git"},
		github.GitCredentialEnv("ghp_supersecret", matcher))
	assert.NoError(t, err)

	for _, source := range sources.Sources() {
		assert.False(t, strings.Contains(source, "ghp_supersecret"),
			"token leaked into source URI: %s", source)
	}
}

func TestForURIsRejectsGitRemoteHelperURI(t *testing.T) {
	l, _ := ui.NewForTesting()

	_, err := ForURIs(l, "testdir", "testenv", []string{"zzq::x.git"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remote helpers are not supported")
}

func TestForURIsRejectsUnsupportedScheme(t *testing.T) {
	l, _ := ui.NewForTesting()

	_, err := ForURIs(l, "testdir", "testenv", []string{"invalid://not-a-valid-source"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported source")
}
