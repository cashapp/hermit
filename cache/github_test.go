package cache

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestGetGitHubReleaseInfoRequiresGitHubHost(t *testing.T) {
	_, ok := getGitHubReleaseInfo("https://github.com/owner/repo/releases/download/v1.0.0/tool.tar.gz")
	assert.True(t, ok)

	_, ok = getGitHubReleaseInfo("https://githubXcom/owner/repo/releases/download/v1.0.0/tool.tar.gz")
	assert.False(t, ok)
}
