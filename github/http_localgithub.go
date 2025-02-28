//go:build localgithub

package github

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cashapp/hermit/github/auth"
	"github.com/cashapp/hermit/ui"
)

// AuthenticatedTransport returns a HTTP transport that will inject a
// GitHub authentication token into any requests and handle test-specific URL overrides,
// fetching the token from the provided auth.Provider only when needed.
func AuthenticatedTransport(ui *ui.UI, transport http.RoundTripper, provider auth.Provider) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &testGitHubProviderClient{
		rt:       transport,
		provider: provider,
		ui:       ui,
	}
}

type testGitHubProviderClient struct {
	ui       *ui.UI
	rt       http.RoundTripper
	provider auth.Provider
}

func (g *testGitHubProviderClient) RoundTrip(req *http.Request) (*http.Response, error) {
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

	// Only fetch the token when needed
	if g.provider != nil {
		token, err := g.provider.GetToken()
		if err == nil && token != "" {
			req.Header.Set("Authorization", "token "+token)
		}
	}

	resp, err := g.rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
