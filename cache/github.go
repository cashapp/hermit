package cache

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/pkg/errors"

	"github.com/cashapp/hermit/github"
)

// matches: https://github.com/{OWNER}/{REPO}/releases/download/{TAG}/{ASSET}
var githubRe = regexp.MustCompile(`^https\://github.com/([^/]+)/([^/]+)/releases/download/([^/]+)/([^/]+)$`)

// GitHubPrivateReleaseDownloadStrategy can download private release assets from GitHub using an authenticated GitHub client.
func GitHubPrivateReleaseDownloadStrategy(client *github.Client) DownloadStrategy {
	return func(ctx context.Context, url string) (*http.Response, error) {
		info, ok := getGitHubReleaseInfo(url)
		if !ok {
			return nil, errors.Errorf("not a GitHub URL: %s", url)
		}
		return downloadGHPrivate(client, info)
	}
}

func downloadGHPrivate(client *github.Client, ghi *githubReleaseInfo) (response *http.Response, err error) {
	r, err := client.Releases(fmt.Sprintf("%s/%s", ghi.owner, ghi.repo))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	asset, err := getAssetURL(r, ghi.tag, ghi.asset)
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

func getAssetURL(releases []github.Release, tag, assetName string) (github.Asset, error) {
	for _, r := range releases {
		if r.TagName != tag {
			continue
		}
		for _, a := range r.Assets {
			if a.Name == assetName {
				return a, nil
			}
		}
	}
	return github.Asset{}, errors.Errorf("cannot find asset %s %s", tag, assetName)
}
