package github

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func matchAll(_, _ string) bool { return true }

func expectedHeaderEnv(token string) []string {
	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + auth,
	}
}

func TestGitCredentialEnv(t *testing.T) {
	for _, test := range []struct {
		name     string
		token    string
		matcher  RepoMatcher
		uri      string
		expected []string
	}{
		{"MatchingRepo", "tok", matchAll, "https://github.com/cashapp/hermit.git", expectedHeaderEnv("tok")},
		{"NonMatchingRepo", "tok", func(_, _ string) bool { return false },
			"https://github.com/cashapp/hermit.git", nil},
		{"NoToken", "", matchAll, "https://github.com/cashapp/hermit.git", nil},
		{"SSHURL", "tok", matchAll, "git@github.com:cashapp/hermit.git", nil},
		{"NonGitHubHost", "tok", matchAll, "https://example.com/cashapp/hermit.git", nil},
		{"HostSpoofViaUserinfo", "tok", matchAll, "https://github.com@evil.com/o/r.git", nil},
		{"PlainHTTP", "tok", matchAll, "http://github.com/cashapp/hermit.git", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, GitCredentialEnv(test.token, test.matcher)(test.uri))
		})
	}
}

func TestGitCredentialEnvDoesNotLeakTokenIntoURI(t *testing.T) {
	uri := "https://github.com/cashapp/hermit.git"
	env := GitCredentialEnv("ghp_supersecret", matchAll)(uri)
	assert.NotZero(t, env)
	for _, v := range env {
		assert.False(t, strings.Contains(v, "ghp_supersecret"),
			"raw token must be base64 encoded, not embedded verbatim: %s", v)
	}
}
