package github

import (
	"net/url"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
)

// isGitHubHTTPSURL checks if a URL is an HTTPS URL for a configured GitHub host and returns owner/repo if it is.
func isGitHubHTTPSURL(u *url.URL, host string) (owner, repo string, ok bool) {
	if u.Scheme != "https" || NormalizeHost(u.Host) != NormalizeHost(host) {
		return "", "", false
	}

	parts := strings.Split(strings.TrimSuffix(u.Path, ".git"), "/")
	if len(parts) != 3 {
		return "", "", false
	}

	return parts[1], parts[2], true
}

// isGitHubSSHURL checks if a URL is a GitHub SSH URL (git@github.com:owner/repo.git).
func isGitHubSSHURL(uri string) bool {
	return strings.HasPrefix(uri, "git@github.com:")
}

// AuthenticatedURLRewriter rewrites HTTPS URLs for a configured GitHub host to
// include an auth token if they match the provided pattern.
func AuthenticatedURLRewriter(host string, token redact.Secret, matcher RepoMatcher) func(uri redact.URL) (redact.URL, error) {
	host = NormalizeHost(host)
	return func(repo redact.URL) (redact.URL, error) {
		// Pass through SSH URLs unchanged. Users should configure SSH authentication separately.
		if isGitHubSSHURL(repo.Reveal()) {
			return repo, nil
		}

		u, err := url.Parse(repo.Reveal())
		if err != nil {
			if uerr, ok := err.(*url.Error); ok { //nolint:errorlint
				return "", errors.Errorf("invalid URL %q: %s", repo, uerr.Err)
			}
			return "", errors.Errorf("invalid URL %q", repo)
		}

		owner, repoName, ok := isGitHubHTTPSURL(u, host)
		if !ok || token == "" {
			return repo, nil
		}
		if matcher(owner, repoName) {
			u.User = url.UserPassword("x-access-token", token.Reveal())
			return redact.URL(u.String()), nil
		}
		return repo, nil
	}
}
