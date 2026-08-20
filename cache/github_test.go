package cache

import (
	"net/http"
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
)

type testPackageSource struct{}

func (testPackageSource) OpenLocal(*Cache, string) (*os.File, error) { return nil, nil }
func (testPackageSource) Download(*ui.Task, *Cache, string) (string, string, string, error) {
	return "", "", "", nil
}
func (testPackageSource) ETag(*ui.Task) (string, error) { return "", nil }
func (testPackageSource) Validate() error               { return nil }

func TestGitHubSourceSelector(t *testing.T) {
	matcher, err := github.GlobRepoMatcher([]string{"mycompany/*"})
	assert.NoError(t, err)
	fallback := testPackageSource{}
	selector := GitHubSourceSelectorForHosts(func(*http.Client, redact.URL) (PackageSource, error) {
		return fallback, nil
	}, github.NewWithHosts(nil, []github.HostConfig{{WebHost: "github.com"}, {WebHost: "mycompany.ghe.com"}}), []GitHubHostMatcher{{Host: "mycompany.ghe.com", Match: matcher}})

	source, err := selector(nil, redact.URL("https://mycompany.ghe.com/mycompany/tool/releases/download/v1/tool.tar.gz"))
	assert.NoError(t, err)
	_, ok := source.(*githubReleaseSource)
	assert.True(t, ok)

	source, err = selector(nil, redact.URL("https://github.com/mycompany/tool/releases/download/v1/tool.tar.gz"))
	assert.NoError(t, err)
	_, ok = source.(testPackageSource)
	assert.True(t, ok)

	source, err = selector(nil, redact.URL("https://mycompany.ghe.com/other/tool/releases/download/v1/tool.tar.gz"))
	assert.NoError(t, err)
	_, ok = source.(testPackageSource)
	assert.True(t, ok)
}

func TestGitHubSourceSelectorRecognizesGHEByDefault(t *testing.T) {
	fallback := testPackageSource{}
	selector := GitHubSourceSelectorForHosts(func(*http.Client, redact.URL) (PackageSource, error) {
		return fallback, nil
	}, github.NewWithHosts(nil, []github.HostConfig{{WebHost: "github.com"}}), nil)

	source, err := selector(nil, redact.URL("https://another.ghe.com/other/tool/releases/download/v1/tool.tar.gz"))
	assert.NoError(t, err)
	_, ok := source.(*githubReleaseSource)
	assert.True(t, ok)
}

func TestGitHubSourceSelectorRecognizesConfiguredEnterpriseHost(t *testing.T) {
	matcher, err := github.GlobRepoMatcher([]string{"owner/*"})
	assert.NoError(t, err)
	fallback := testPackageSource{}
	selector := GitHubSourceSelectorForHosts(func(*http.Client, redact.URL) (PackageSource, error) {
		return fallback, nil
	}, github.NewWithHosts(nil, []github.HostConfig{{WebHost: "github.internal.example"}}), []GitHubHostMatcher{{Host: "github.internal.example", Match: matcher}})

	source, err := selector(nil, redact.URL("https://github.internal.example/owner/tool/releases/download/v1/tool.tar.gz"))
	assert.NoError(t, err)
	_, ok := source.(*githubReleaseSource)
	assert.True(t, ok)
}

func TestGetGitHubReleaseInfoRejectsQueryAndFragment(t *testing.T) {
	for _, uri := range []string{
		"https://github.com/owner/repo/releases/download/v1/tool.tar.gz?sig=value",
		"https://github.com/owner/repo/releases/download/v1/tool.tar.gz#fragment",
	} {
		t.Run(uri, func(t *testing.T) {
			_, ok := getGitHubReleaseInfo(uri)
			assert.False(t, ok)
		})
	}
}

func TestGetGitHubReleaseInfoNormalizesHost(t *testing.T) {
	info, ok := getGitHubReleaseInfo("https://MYCOMPANY.GHE.COM:443/owner/repo/releases/download/v1/tool.tar.gz")
	assert.True(t, ok)
	assert.Equal(t, "mycompany.ghe.com", info.host)
}

func TestGitHubSourceSelectorRejectsUnknownHost(t *testing.T) {
	matcher, err := github.GlobRepoMatcher([]string{"owner/*"})
	assert.NoError(t, err)
	fallback := testPackageSource{}
	selector := GitHubSourceSelector(func(*http.Client, redact.URL) (PackageSource, error) {
		return fallback, nil
	}, github.NewWithHosts(nil, nil), matcher)

	source, err := selector(nil, redact.URL("https://githubXcom/owner/repo/releases/download/v1/tool.tar.gz"))
	assert.NoError(t, err)
	_, ok := source.(testPackageSource)
	assert.True(t, ok)
}
