package github

import (
	"net/http"
)

// TokenAuthenticatedTransport returns a HTTP transport that will inject a
// GitHub authentication token into any requests to github.com.
//
// Conceptually similar to
// https://github.com/google/go-github/blob/d23570d44313ca73dbcaadec71fc43eca4d29f8b/github/github.go#L841-L875
func TokenAuthenticatedTransport(transport http.RoundTripper, token string) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &githubAuthenticatedHTTPClient{rt: transport, token: token}
}

type githubAuthenticatedHTTPClient struct {
	token string
	rt    http.RoundTripper
}

func (g *githubAuthenticatedHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context()) // The stdlib docs recommend not mutating the request in place.
	if (req.URL.Host == "github.com" || req.URL.Host == "api.github.com") && g.token != "" {
		req.Header.Set("Authorization", "token "+g.token)
	}
	return g.rt.RoundTrip(req)
}
