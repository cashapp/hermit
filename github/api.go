// Package github implements a client for GitHub that includes the minimum set
// of functions required by Hermit.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github/auth"
	"github.com/cashapp/hermit/ui"
)

const (
	// gitHubHost is the hostname for GitHub
	gitHubHost = "github.com"
)

// Repo information.
type Repo struct {
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}

// Release is a minimal type for GitHub releases meta information retrieved via the GitHub API.
//
// See https://docs.github.com/en/rest/reference/repos#list-releases
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is a minimal type for assets in the GitHub releases meta information retrieved via the GitHub API.
//
// See https://docs.github.com/en/rest/reference/repos#list-releases
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Client for GitHub.
type Client struct {
	cache  sync.Map
	client *http.Client
}

// New creates a new GitHub API client.
func New(ui *ui.UI, client *http.Client, provider auth.Provider) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	if provider == nil {
		client = http.DefaultClient
	} else {
		client = &http.Client{Transport: AuthenticatedTransport(ui, client.Transport, provider)}
	}
	return &Client{client: client}
}

// ProjectForURL returns the <repo>/<project> for the given URL if it is a GitHub project.
func (a *Client) ProjectForURL(sourceURL string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	if u.Host != gitHubHost {
		return ""
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[1:3], "/")
}

// Repo information.
func (a *Client) Repo(repo string) (*Repo, error) {
	response := &Repo{}
	url := "https://api.github.com/repos/" + repo
	return response, a.decode(url, response)
}

// Release attempts to fetch Release info for a tag.
func (a *Client) Release(repo, tag string) (*Release, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/tags/" + tag
	release := &Release{}
	return release, a.decode(url, release)
}

// LatestRelease details for a GitHub repository.
func (a *Client) LatestRelease(repo string) (*Release, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	release := &Release{}
	return release, a.decode(url, release)
}

// Releases for a particular repo. If limit is 0, fetches all releases.
func (a *Client) Releases(repo string, limit int) (releases []*Release, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	// Paginate.
	for n := 1; n < 100; n++ {
		var page []*Release
		url = fmt.Sprintf("%s?per_page=100&page=%d", url, n)
		err = a.decode(url, &page)
		if err != nil {
			return nil, err
		}
		releases = append(releases, page...)
		if limit > 0 && len(releases) >= limit {
			return releases[:limit], nil
		}
		if len(page) < 100 {
			return releases, nil
		}
	}
	return nil, errors.Errorf("could not fully paginate over GitHub releases in %s, too many results", repo)
}

// Download creates a download request for retrieving a release asset from GitHub.
func (a *Client) Download(asset Asset) (resp *http.Response, err error) {
	req, err := a.request("GET", asset.URL, http.Header{
		"Accept": []string{"application/octet-stream"},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return a.client.Do(req)
}

// ETag issues a HEAD request for an Asset and returns its ETag.
func (a *Client) ETag(asset Asset) (etag string, err error) {
	req, err := a.request("HEAD", asset.URL, http.Header{
		"Accept": []string{"application/octet-stream"},
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", errors.Wrapf(err, "failed to retrieve ETag")
	}
	return resp.Header.Get("ETag"), nil
}

func (a *Client) decode(url string, dest interface{}) error {
	var body *bytes.Reader
	ibody, ok := a.cache.Load(url)
	if ok {
		body = bytes.NewReader(ibody.([]byte))
	} else {
		req, err := a.request("GET", url, http.Header{})
		if err != nil {
			return errors.Wrap(err, url)
		}
		resp, err := a.client.Do(req)
		if err != nil {
			return errors.Wrap(err, url)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return errors.Errorf("%s: GitHub API request failed with %s", url, resp.Status)
		}
		w := &bytes.Buffer{}
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			return errors.Wrap(err, url)
		}
		a.cache.Store(url, w.Bytes())
		body = bytes.NewReader(w.Bytes())
	}
	dec := json.NewDecoder(body)
	err := dec.Decode(dest)
	if err != nil {
		return errors.Wrap(err, url)
	}
	return nil
}

func (a *Client) request(method string, url string, headers http.Header) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil) // nolint: noctx
	if err != nil {
		return nil, errors.WithStack(err)
	}
	headers = headers.Clone()
	req.Header = headers
	return req, nil
}
