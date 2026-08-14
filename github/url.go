package github

import (
	"net/url"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources"
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
func isGitHubSSHURL(uri sources.Source) bool {
	return strings.HasPrefix(uri.Get(), "git@github.com:")
}

// AuthenticatedURLRewriter rewrites GitHub URLs to include an auth token if they match the provided pattern
func AuthenticatedURLRewriter(token string, matcher RepoMatcher) sources.URLRewriter {
	return func(repo sources.Source) (sources.Source, error) {
		// Pass through SSH URLs unchanged
		if isGitHubSSHURL(repo) {
			return repo, nil
		}

		u, err := url.Parse(repo.Get())
		if err != nil {
			return sources.Source{}, errors.Errorf("invalid GitHub source %q", repo)
		}

		owner, repoName, ok := isGitHubHTTPSURL(u)
		if !ok || token == "" {
			return repo, nil
		}
		if matcher(owner, repoName) {
			u.User = url.UserPassword("x-access-token", token)
			return sources.NewSource(u.String()), nil
		}
		return repo, nil
	}
}
