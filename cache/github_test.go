package cache

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/sources"
)

func TestGetGitHubReleaseInfoRequiresGitHubHost(t *testing.T) {
	_, ok := getGitHubReleaseInfo(sources.NewSource("https://github.com/owner/repo/releases/download/v1.0.0/tool.tar.gz"))
	assert.True(t, ok)

	_, ok = getGitHubReleaseInfo(sources.NewSource("https://githubXcom/owner/repo/releases/download/v1.0.0/tool.tar.gz"))
	assert.False(t, ok)
}
