package redact_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/redact"
)

func TestURLDisplaysWithoutCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"UserAndPassword", "https://x-access-token:sekret@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"UserOnly", "https://sekret@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"LiteralAtInPassword", "https://user:p@ss@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"NoCredentials", "https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"AtInPath", "https://github.com/owner/repo@v1.0", "https://github.com/owner/repo@v1.0"},
		{"AtInQuery", "https://example.com?user=@foo", "https://example.com?user=@foo"},
		{"SCPLike", "git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"EnvScheme", "env:///bin/packages", "env:///bin/packages"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			url := redact.URL(test.in)
			assert.Equal(t, test.want, url.String())
			assert.Equal(t, test.in, url.Reveal())
		})
	}
}

func TestURLFormattingRedacts(t *testing.T) {
	url := redact.URL("https://x-access-token:sekret@github.com/owner/repo.git")
	assert.Equal(t, "https://github.com/owner/repo.git", fmt.Sprintf("%v", url))
	assert.Equal(t, "https://github.com/owner/repo.git", fmt.Sprintf("%#v", url))
	assert.Equal(t, `"https://github.com/owner/repo.git"`, fmt.Sprintf("%q", url))
}

func TestURLSerialisesRaw(t *testing.T) {
	url := redact.URL("https://user:sekret@example.com/repo.git")
	data, err := json.Marshal(url)
	assert.NoError(t, err)
	assert.Equal(t, `"https://user:sekret@example.com/repo.git"`, string(data))
	var back redact.URL
	assert.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, url, back)
}

func TestSecret(t *testing.T) {
	secret := redact.Secret("sekret")
	assert.Equal(t, "[redacted]", secret.String())
	assert.Equal(t, "[redacted]", fmt.Sprintf("%v", secret))
	assert.Equal(t, "[redacted]", fmt.Sprintf("%#v", secret))
	assert.Equal(t, "sekret", secret.Reveal())
	assert.Equal(t, "", redact.Secret("").String())
}

func TestArgsRoundTrip(t *testing.T) {
	args := append(redact.Args("git", "clone"), redact.URL("https://user:sekret@example.com/repo.git"))
	assert.Equal(t, []string{"git", "clone", "https://user:sekret@example.com/repo.git"}, redact.Reveal(args))
	assert.Equal(t, "git", args[0].String())
}

func TestScrubber(t *testing.T) {
	assert.Zero(t, redact.Scrubber(redact.Args("git", "clone")))

	args := append(redact.Args("git", "clone"), redact.URL("https://user:sekret@example.com/repo.git"))
	scrubber := redact.Scrubber(args)
	assert.Equal(t,
		"fatal: unable to access 'https://example.com/repo.git/'",
		scrubber.Replace("fatal: unable to access 'https://user:sekret@example.com/repo.git/'"))
}
