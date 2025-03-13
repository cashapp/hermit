package github

import (
	"net/url"
	"strings"

	"github.com/cashapp/hermit/errors"
)

// AuthenticatedURLRewriter rewrites GitHub URLs to include an auth token if they match the provided pattern
func AuthenticatedURLRewriter(token string, matcher RepoMatcher) func(uri string) (string, error) {
	return func(repo string) (string, error) {
		owner, repoName, ok := isGitHubHTTPSURL(repo)
		if !ok || token == "" {
			return repo, nil
		}
		if matcher(owner, repoName) {
			u, err := url.Parse(repo)
			if err != nil {
				return "", errors.WithStack(err)
			}
			u.User = url.UserPassword("x-access-token", token)
			return u.String(), nil
		}
		return repo, nil
	}
}

// isGitHubHTTPSURL checks if a URL is a GitHub HTTPS URL and returns owner/repo if it is
func isGitHubHTTPSURL(uri string) (owner, repo string, ok bool) {
	if !strings.HasPrefix(uri, "https://github.com/") {
		return "", "", false
	}

	u, err := url.Parse(uri)
	if err != nil {
		return "", "", false
	}

	parts := strings.Split(strings.TrimSuffix(u.Path, ".git"), "/")
	if len(parts) != 3 {
		return "", "", false
	}

	return parts[1], parts[2], true
}
