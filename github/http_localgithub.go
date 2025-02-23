//go:build localgithub

package github

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cashapp/hermit/ui"
)

// TokenAuthenticatedTransport returns a HTTP transport that will inject a
// GitHub authentication token into any requests and handle test-specific URL overrides.
func TokenAuthenticatedTransport(ui *ui.UI, transport http.RoundTripper, token string) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &testGitHubClient{
		rt:    transport,
		token: token,
		ui:    ui,
	}
}

type testGitHubClient struct {
	ui    *ui.UI
	rt    http.RoundTripper
	token string
}

func (g *testGitHubClient) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if this is a GitHub request or if it's already been rewritten to our mock server
	isGitHubRequest := req.URL.Host == "github.com" || req.URL.Host == "api.github.com"
	isMockServerRequest := strings.Contains(req.URL.String(), os.Getenv("HERMIT_GITHUB_BASE_URL"))

	if !isGitHubRequest && !isMockServerRequest {
		return g.rt.RoundTrip(req)
	}

	baseURL := os.Getenv("HERMIT_GITHUB_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("HERMIT_GITHUB_BASE_URL must be set when making GitHub requests with localgithub build tag")
	}

	g.ui.Tracef("Using mock github server URL: %s", baseURL)
	req = req.Clone(req.Context())

	mockURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid HERMIT_GITHUB_BASE_URL: %v", err)
	}

	// Rewrite GitHub URLs to use the mock server if not already using it
	if !isMockServerRequest {
		req.URL.Scheme = mockURL.Scheme
		req.URL.Host = mockURL.Host
	}

	if g.token != "" {
		req.Header.Set("Authorization", "token "+g.token)
	}

	resp, err := g.rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
