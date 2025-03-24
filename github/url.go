package github

import (
	"net/url"
	"strings"

	"github.com/cashapp/hermit/errors"
)

// isGitHubHTTPSURL checks if a URL is a GitHub HTTPS URL and returns owner/repo if it is
func isGitHubHTTPSURL(u *url.URL) (owner, repo string, ok bool) {
	if u.Scheme != "https" || u.Host != "github.com" {
		return "", "", false
	}

	parts := strings.Split(strings.TrimSuffix(u.Path, ".git"), "/")
	if len(parts) != 3 {
		return "", "", false
	}

	return parts[1], parts[2], true
}

// AuthenticatedURLRewriter rewrites GitHub URLs to include an auth token if they match the provided pattern
func AuthenticatedURLRewriter(token string, matcher RepoMatcher) func(uri string) (string, error) {
	return func(repo string) (string, error) {
		u, err := url.Parse(repo)
		if err != nil {
			return "", errors.WithStack(err)
		}

		owner, repoName, ok := isGitHubHTTPSURL(u)
		if !ok || token == "" {
			return repo, nil
		}
		if matcher(owner, repoName) {
			u.User = url.UserPassword("x-access-token", token)
			return u.String(), nil
		}
		return repo, nil
	}
}
