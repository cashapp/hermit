// Package github implements a client for GitHub that includes the minimum set
// of functions required by Hermit.
package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

// Repo information.
type Repo struct {
	Description string `json:"description"`
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
	client *http.Client
}

// New creates a new GitHub API client.
//
// The passed http.Client should be configured
func New(client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{client: client}
}

// ProjectForURL returns the <repo>/<project> for the given URL if it is a GitHub project.
func (a *Client) ProjectForURL(sourceURL string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	if u.Host != "github.com" {
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

// LatestRelease details for a GitHub repository.
func (a *Client) LatestRelease(repo string) (*Release, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	release := &Release{}
	return release, a.decode(url, release)
}

// Releases for a particular repo.
func (a *Client) Releases(repo string) (releases []Release, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	return releases, a.decode(url, &releases)
}

// PrepareDownload prepares a HTTP client and request for retrieving a file from GitHub.
func (a *Client) PrepareDownload(asset Asset) (client *http.Client, req *http.Request, err error) {
	req, err = a.request(asset.URL, http.Header{
		"Accept": []string{"application/octet-stream"},
	})
	return a.client, req, err
}

func (a *Client) decode(url string, dest interface{}) error {
	req, err := a.request(url, http.Header{})
	if err != nil {
		return errors.WithStack(err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errors.Errorf("GitHub API request failed with %s", resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(dest)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (a *Client) request(url string, headers http.Header) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil) // nolint: noctx
	if err != nil {
		return nil, errors.WithStack(err)
	}
	headers = headers.Clone()
	req.Header = headers
	return req, nil
}
