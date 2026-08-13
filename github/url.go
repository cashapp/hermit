package github

import (
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/cashapp/hermit/util"
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

// GitCredentialEnv authenticates matching GitHub repositories out of band: a token
// embedded in the URL leaks into logs, the source directory name and .git/config.
func GitCredentialEnv(token string, matcher RepoMatcher) func(uri string) []string {
	return func(uri string) []string {
		if token == "" || isGitHubSSHURL(uri) {
			return nil
		}
		u, err := url.Parse(uri)
		if err != nil {
			return nil
		}
		owner, repoName, ok := isGitHubHTTPSURL(u)
		if !ok || !matcher(owner, repoName) {
			return nil
		}
		auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		return util.GitConfigEnv(
			"http.https://"+gitHubHost+"/.extraheader", "Authorization: Basic "+auth)
	}
}
