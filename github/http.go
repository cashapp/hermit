package github

import (
	"net/http"
	"strings"
	"sync"

	"github.com/cashapp/hermit/redact"
)

// TokenAuthenticatedTransport returns a HTTP transport that will inject the
// configured GitHub authentication token into requests to each configured GitHub
// web host and API host. Tokens for non-GitHub.com hosts are fetched from gh if
// not explicitly configured.
//
// Conceptually similar to
// https://github.com/google/go-github/blob/d23570d44313ca73dbcaadec71fc43eca4d29f8b/github/github.go#L841-L875
func TokenAuthenticatedTransport(transport http.RoundTripper, hosts []HostConfig) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	auth := &githubAuthTokenSource{
		tokensByWebHost: map[string]redact.Secret{},
		knownWebHosts:   map[string]bool{},
		ghTokens:        map[string]func() (redact.Secret, error){},
	}
	for _, host := range hosts {
		webHost := NormalizeHost(host.WebHost)
		auth.knownWebHosts[webHost] = true
		if host.Token != "" {
			auth.tokensByWebHost[webHost] = host.Token
		}
	}
	return &githubAuthenticatedHTTPClient{rt: transport, auth: auth}
}

type githubAuthenticatedHTTPClient struct {
	auth *githubAuthTokenSource
	rt   http.RoundTripper
}

func (g *githubAuthenticatedHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context()) // The stdlib docs recommend not mutating the request in place.
	token, err := g.auth.tokenForRequestHost(req.URL.Host)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token.Reveal())
	}
	return g.rt.RoundTrip(req)
}

type githubAuthTokenSource struct {
	mu              sync.Mutex
	tokensByWebHost map[string]redact.Secret
	knownWebHosts   map[string]bool
	ghTokens        map[string]func() (redact.Secret, error)
}

func (g *githubAuthTokenSource) tokenForRequestHost(requestHost string) (redact.Secret, error) {
	webHost, ok := g.webHostForRequestHost(requestHost)
	if !ok {
		return "", nil
	}

	g.mu.Lock()
	if token := g.tokensByWebHost[webHost]; token != "" {
		g.mu.Unlock()
		return token, nil
	}
	if webHost == gitHubHost {
		g.mu.Unlock()
		return "", nil
	}
	tokenFromCLI, ok := g.ghTokens[webHost]
	if !ok {
		tokenFromCLI = sync.OnceValues(func() (redact.Secret, error) {
			return TokenFromCLI(webHost)
		})
		g.ghTokens[webHost] = tokenFromCLI
	}
	g.mu.Unlock()
	return tokenFromCLI()
}

func (g *githubAuthTokenSource) webHostForRequestHost(requestHost string) (string, bool) {
	requestHost = NormalizeHost(requestHost)
	if strings.HasPrefix(requestHost, "api.") {
		webHost := strings.TrimPrefix(requestHost, "api.")
		if g.knownWebHosts[webHost] || IsGitHubHost(webHost) {
			return webHost, true
		}
	}
	if g.knownWebHosts[requestHost] || IsGitHubHost(requestHost) {
		return requestHost, true
	}
	return "", false
}
