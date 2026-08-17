package github

import (
	"net/url"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
)

// isGitHubHTTPSURL checks if a URL is a GitHub HTTPS URL and returns owner/repo if it is
func isGitHubHTTPSURL(u *url.URL) (owner, repo string, ok bool) {
	if u.Scheme != "https" || u.Host != gitHubHost {
		return "", "", false
	}

	parts := strings.Split(strings.TrimSuffix(u.Path, ".git"), "/")
	if len(parts) != 3 {
		return "", "", false
	}

	return parts[1], parts[2], true
}

// isGitHubSSHURL checks if a URL is a GitHub SSH URL (git@github.com:owner/repo.git)
func isGitHubSSHURL(uri string) bool {
	return strings.HasPrefix(uri, "git@github.com:")
}

// AuthenticatedURLRewriter rewrites GitHub URLs to include an auth token if they match the provided pattern
func AuthenticatedURLRewriter(token redact.Secret, matcher RepoMatcher) func(uri redact.URL) (redact.URL, error) {
	return func(repo redact.URL) (redact.URL, error) {
		// Pass through SSH URLs unchanged
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

		owner, repoName, ok := isGitHubHTTPSURL(u)
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
