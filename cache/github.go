package cache

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/ui"
)

// matches: https://github.com/{OWNER}/{REPO}/releases/download/{TAG}/{ASSET}
var githubRe = regexp.MustCompile(`^https\://github.com/([^/]+)/([^/]+)/releases/download/([^/]+)/([^/]+)$`)

// RepoMatcher is used to determine which repositories will use authenticated requests.
type RepoMatcher func(owner, repo string) bool

// GitHubSourceSelector can download private release assets from GitHub using an authenticated GitHub client.
func GitHubSourceSelector(getSource PackageSourceSelector, ghclient *github.Client, match RepoMatcher) PackageSourceSelector {
	return func(client *http.Client, uri string) (PackageSource, error) {
		info, ok := getGitHubReleaseInfo(uri)
		if !ok || match == nil || !match(info.owner, info.repo) {
			return getSource(client, uri)
		}
		return &githubReleaseSource{url: uri, info: info, ghclient: ghclient}, nil
	}
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

func (g *githubReleaseSource) Download(b *ui.Task, c *Cache, checksum string) (path string, etag string, err error) {
	response, err := downloadGHPrivate(g.ghclient, g.info)
	if err != nil {
		return "", "", err
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
	return g.ghclient.ETag(asset)
}

func (g *githubReleaseSource) Validate() error {
	asset, err := g.getAsset()
	if err != nil {
		return err
	}
	_, err = g.ghclient.ETag(asset)
	return errors.WithStack(err)
}

func (g *githubReleaseSource) getAsset() (github.Asset, error) {
	release, err := g.ghclient.Release(fmt.Sprintf("%s/%s", g.info.owner, g.info.repo), g.info.tag)
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
	release, err := client.Release(fmt.Sprintf("%s/%s", ghi.owner, ghi.repo), ghi.tag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	asset, err := getAssetURL(release, ghi.asset)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp, err := client.Download(asset)
	if err != nil {
		return nil, errors.Wrap(err, "GitHub release API download failed")
	}
	return resp, nil
}

type githubReleaseInfo struct {
	owner, repo, tag, asset string
}

func getGitHubReleaseInfo(uri string) (*githubReleaseInfo, bool) {
	g := &githubReleaseInfo{}
	m := githubRe.FindStringSubmatch(uri)
	if len(m) != 5 {
		return nil, false
	}
	var err error
	if g.owner, err = url.PathUnescape(m[1]); err != nil {
		return nil, false
	}
	if g.repo, err = url.PathUnescape(m[2]); err != nil {
		return nil, false
	}
	if g.tag, err = url.PathUnescape(m[3]); err != nil {
		return nil, false
	}
	if g.asset, err = url.PathUnescape(m[4]); err != nil {
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
