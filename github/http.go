//go:build !localgithub

package github

import (
	"net/http"

	"github.com/cashapp/hermit/github/auth"
	"github.com/cashapp/hermit/ui"
)

// AuthenticatedTransport returns a HTTP transport that will inject a
// GitHub authentication token into any requests to github.com, fetching the token
// from the provided auth.Provider only when needed.
func AuthenticatedTransport(_ *ui.UI, transport http.RoundTripper, provider auth.Provider) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &githubProviderAuthenticatedHTTPClient{rt: transport, provider: provider}
}

type githubProviderAuthenticatedHTTPClient struct {
	provider auth.Provider
	rt       http.RoundTripper
}

func (g *githubProviderAuthenticatedHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context()) // The stdlib docs recommend not mutating the request in place.
	if req.URL.Host == "github.com" || req.URL.Host == "api.github.com" {
		// Only fetch the token when needed for GitHub API requests
		token, err := g.provider.GetToken()
		if err == nil && token != "" {
			req.Header.Set("Authorization", "token "+token)
		}
	}
	return g.rt.RoundTrip(req)
}
