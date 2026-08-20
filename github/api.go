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
	"os/exec"
	"strings"
	"sync"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
)

const (
	// gitHubHost is the hostname for GitHub.com.
	gitHubHost = "github.com"
)

// HostConfig configures a GitHub host.
type HostConfig struct {
	// WebHost is the hostname users see in source URLs, for example
	// "github.com" or "mycompany.ghe.com".
	WebHost string
	// Token is the token to use for this host and API.
	Token redact.Secret
}

// APIBaseURL returns the REST API base URL for a GitHub web host.
func APIBaseURL(webHost string) string {
	return "https://api." + NormalizeHost(webHost)
}

// NormalizeHost normalizes a configured GitHub web host.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return gitHubHost
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.Trim(strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"), "/"))
	}
	if u.Port() == "" || u.Port() == "443" {
		return strings.ToLower(u.Hostname())
	}
	return strings.ToLower(u.Host)
}

// IsGitHubHost returns true for hosts Hermit treats as GitHub-compatible by default.
func IsGitHubHost(host string) bool {
	host = NormalizeHost(host)
	return host == gitHubHost || (strings.HasSuffix(host, ".ghe.com") && len(strings.Split(host, ".")) == 3)
}

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
	hosts  map[string]HostConfig
}

// NewWithHosts creates a new GitHub API client for one or more GitHub hosts.
func NewWithHosts(client *http.Client, hosts []HostConfig) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	if len(hosts) == 0 {
		hosts = []HostConfig{{WebHost: gitHubHost}}
	}

	normalized := make(map[string]HostConfig, len(hosts))
	for _, host := range hosts {
		host.WebHost = NormalizeHost(host.WebHost)
		normalized[host.WebHost] = host
	}

	authenticatedClient := *client
	authenticatedClient.Transport = TokenAuthenticatedTransportForHosts(client.Transport, hosts)
	return &Client{client: &authenticatedClient, hosts: normalized}
}

// SupportsHost returns true if this client supports the given GitHub web host.
func (a *Client) SupportsHost(host string) bool {
	host = NormalizeHost(host)
	if IsGitHubHost(host) {
		return true
	}
	_, ok := a.hosts[host]
	return ok
}

// ProjectForURL returns the configured GitHub web host and <repo>/<project>
// for the given URL if it is a configured GitHub project.
func (a *Client) ProjectForURL(sourceURL string) (host, project string) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", ""
	}
	host = NormalizeHost(u.Host)
	if !a.SupportsHost(host) {
		return "", ""
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) < 3 {
		return "", ""
	}
	return host, strings.Join(parts[1:3], "/")
}

// Repo information.
func (a *Client) Repo(host, repo string) (*Repo, error) {
	response := &Repo{}
	url, err := a.apiURL(host, "/repos/"+repo)
	if err != nil {
		return nil, err
	}
	return response, a.decode(url, response)
}

// Release attempts to fetch Release info for a tag from the configured GitHub host.
func (a *Client) Release(host, repo, tag string) (*Release, error) {
	url, err := a.apiURL(host, "/repos/"+repo+"/releases/tags/"+tag)
	if err != nil {
		return nil, err
	}
	release := &Release{}
	return release, a.decode(url, release)
}

// LatestRelease details for a GitHub repository from the configured GitHub host.
func (a *Client) LatestRelease(host, repo string) (*Release, error) {
	url, err := a.apiURL(host, "/repos/"+repo+"/releases/latest")
	if err != nil {
		return nil, err
	}
	release := &Release{}
	return release, a.decode(url, release)
}

// Releases for a particular repo. If limit is 0, fetches all releases.
func (a *Client) Releases(host, repo string, limit int) (releases []*Release, err error) {
	baseURL, err := a.apiURL(host, fmt.Sprintf("/repos/%s/releases", repo))
	if err != nil {
		return nil, err
	}
	// Paginate.
	for n := 1; n < 100; n++ {
		var page []*Release
		url := fmt.Sprintf("%s?per_page=100&page=%d", baseURL, n)
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

// Download creates a download request for retrieving a release asset from the configured GitHub host.
func (a *Client) Download(host string, asset Asset) (resp *http.Response, err error) {
	if err := a.validateAssetURLForHost(host, asset.URL); err != nil {
		return nil, err
	}
	req, err := a.request("GET", asset.URL, http.Header{
		"Accept": []string{"application/octet-stream"},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return a.doForHost(host, req)
}

// ETag issues a HEAD request for an Asset from the configured GitHub host and returns its ETag.
func (a *Client) ETag(host string, asset Asset) (etag string, err error) {
	if err := a.validateAssetURLForHost(host, asset.URL); err != nil {
		return "", err
	}
	req, err := a.request("HEAD", asset.URL, http.Header{
		"Accept": []string{"application/octet-stream"},
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	resp, err := a.doForHost(host, req)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", errors.Wrapf(err, "failed to retrieve ETag")
	}
	return resp.Header.Get("ETag"), nil
}

func (a *Client) apiURL(host, path string) (string, error) {
	host = NormalizeHost(host)
	if !a.SupportsHost(host) {
		return "", errors.Errorf("GitHub host %q is not configured", host)
	}
	return APIBaseURL(host) + path, nil
}

func (a *Client) validateAssetURLForHost(host, rawURL string) error {
	host = NormalizeHost(host)
	if !a.SupportsHost(host) {
		return errors.Errorf("GitHub host %q is not configured", host)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.WithStack(err)
	}
	assetHost := NormalizeHost(u.Host)
	if assetHost != host && assetHost != NormalizeHost("api."+host) {
		return errors.Errorf("GitHub asset URL host %q does not match configured GitHub host %q", assetHost, host)
	}
	return nil
}

func (a *Client) doForHost(host string, req *http.Request) (*http.Response, error) {
	host = NormalizeHost(host)
	client := *a.client
	previousCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := rejectGitHubDotComRedirectForHost(host, req.URL); err != nil {
			return err
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return client.Do(req)
}

func rejectGitHubDotComRedirectForHost(host string, u *url.URL) error {
	if NormalizeHost(host) == gitHubHost {
		return nil
	}
	redirectHost := NormalizeHost(u.Host)
	if redirectHost == gitHubHost || redirectHost == "api."+gitHubHost {
		return errors.Errorf("refusing to follow GitHub Enterprise asset redirect to %s", redirectHost)
	}
	return nil
}

func (a *Client) decode(url string, dest interface{}) error {
	var body *bytes.Reader
	ibody, ok := a.cache.Load(url)
	if ok {
		body = bytes.NewReader(ibody.([]byte)) //nolint
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
	req, err := http.NewRequest(method, url, nil) //nolint: noctx
	if err != nil {
		return nil, errors.WithStack(err)
	}
	headers = headers.Clone()
	req.Header = headers
	return req, nil
}

var ghAuthToken = func(host string) (string, error) {
	out, err := exec.Command("gh", "auth", "token", "-h", host).Output()
	if err != nil {
		return "", errors.Wrapf(err, "GitHub Enterprise host %q requires authentication via the gh CLI; run `gh auth login -h %s`", host, host)
	}
	return strings.TrimSpace(string(out)), nil
}

// TokenFromCLI retrieves a token for an authenticated GitHub Enterprise host.
func TokenFromCLI(host string) (redact.Secret, error) {
	host = NormalizeHost(host)
	token, err := ghAuthToken(host)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", errors.Errorf("gh auth token -h %s returned an empty token", host)
	}
	return redact.Secret(token), nil
}
