package cache

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/ui"
)

// GitHubHostMatcher matches repositories for a specific GitHub host.
type GitHubHostMatcher struct {
	Host  string
	Match github.RepoMatcher
}

// GitHubSourceSelector can download private release assets from GitHub.com using an authenticated GitHub client.
func GitHubSourceSelector(getSource PackageSourceSelector, ghclient *github.Client, match github.RepoMatcher) PackageSourceSelector {
	return GitHubSourceSelectorForHosts(getSource, ghclient, []GitHubHostMatcher{{Host: "github.com", Match: match}})
}

// GitHubSourceSelectorForHosts can download private release assets from GitHub-compatible hosts using an authenticated GitHub client.
func GitHubSourceSelectorForHosts(getSource PackageSourceSelector, ghclient *github.Client, matchers []GitHubHostMatcher) PackageSourceSelector {
	normalizedMatchers := make([]GitHubHostMatcher, 0, len(matchers))
	for _, matcher := range matchers {
		matcher.Host = github.NormalizeHost(matcher.Host)
		normalizedMatchers = append(normalizedMatchers, matcher)
	}
	return func(client *http.Client, uri string) (PackageSource, error) {
		info, ok := getGitHubReleaseInfo(uri)
		if !ok || !ghclient.SupportsHost(info.host) || !matchesGitHubHost(info, normalizedMatchers) {
			return getSource(client, uri)
		}
		return &githubReleaseSource{url: uri, info: info, ghclient: ghclient}, nil
	}
}

func matchesGitHubHost(info *githubReleaseInfo, matchers []GitHubHostMatcher) bool {
	if info.host != "github.com" && strings.HasSuffix(info.host, ".ghe.com") {
		return true
	}
	for _, matcher := range matchers {
		if matcher.Host == info.host && matcher.Match != nil && matcher.Match(info.owner, info.repo) {
			return true
		}
	}
	return false
}

type githubReleaseSource struct {
	info     *githubReleaseInfo
	ghclient *github.Client
	url      string
}

func (g *githubReleaseSource) OpenLocal(c *Cache, checksum string) (*os.File, error) {
	f, err := os.Open(c.Path(checksum, g.url))
	return f, errors.WithStack(err)
}

func (g *githubReleaseSource) Download(b *ui.Task, c *Cache, checksum string) (path string, etag string, actualChecksum string, err error) {
	response, err := downloadGHPrivate(g.ghclient, g.info)
	if err != nil {
		return "", "", "", err
	}
	defer response.Body.Close()
	cachePath := c.Path(checksum, g.url)
	return downloadHTTP(b, response, checksum, g.url, cachePath)
}

func (g *githubReleaseSource) ETag(b *ui.Task) (etag string, err error) {
	asset, err := g.getAsset()
	if err != nil {
		return "", err
	}
	return g.ghclient.ETagForHost(g.info.host, asset)
}

func (g *githubReleaseSource) Validate() error {
	asset, err := g.getAsset()
	if err != nil {
		return err
	}
	_, err = g.ghclient.ETagForHost(g.info.host, asset)
	return errors.WithStack(err)
}

func (g *githubReleaseSource) getAsset() (github.Asset, error) {
	release, err := g.ghclient.ReleaseForHost(g.info.host, fmt.Sprintf("%s/%s", g.info.owner, g.info.repo), g.info.tag)
	if err != nil {
		return github.Asset{}, errors.WithStack(err)
	}
	asset, err := getAssetURL(release, g.info.asset)
	if err != nil {
		return github.Asset{}, errors.WithStack(err)
	}
	return asset, nil
}

func downloadGHPrivate(client *github.Client, ghi *githubReleaseInfo) (response *http.Response, err error) {
	release, err := client.ReleaseForHost(ghi.host, fmt.Sprintf("%s/%s", ghi.owner, ghi.repo), ghi.tag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	asset, err := getAssetURL(release, ghi.asset)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp, err := client.DownloadForHost(ghi.host, asset)
	if err != nil {
		return nil, errors.Wrap(err, "GitHub release API download failed")
	}
	return resp, nil
}

type githubReleaseInfo struct {
	host, owner, repo, tag, asset string
}

func getGitHubReleaseInfo(uri string) (*githubReleaseInfo, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" {
		return nil, false
	}
	g := &githubReleaseInfo{host: u.Host}
	if g.owner, err = url.PathUnescape(parts[0]); err != nil {
		return nil, false
	}
	if g.repo, err = url.PathUnescape(parts[1]); err != nil {
		return nil, false
	}
	if g.tag, err = url.PathUnescape(parts[4]); err != nil {
		return nil, false
	}
	if g.asset, err = url.PathUnescape(parts[5]); err != nil {
		return nil, false
	}
	return g, true
}

func getAssetURL(r *github.Release, assetName string) (github.Asset, error) {
	candidates := []string{}
	for _, a := range r.Assets {
		if a.Name == assetName {
			return a, nil
		}
		candidates = append(candidates, a.Name)
	}
	return github.Asset{}, errors.Errorf("cannot find asset %s %s, candidates are %s", r.TagName, assetName, strings.Join(candidates, ", "))
}
