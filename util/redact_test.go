package util

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestRedactCredentials(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		expected string
	}{
		{"NoCredentials", "https://github.com/cashapp/hermit.git", "https://github.com/cashapp/hermit.git"},
		{"UserAndPassword",
			"https://x-access-token:ghp_secret@github.com/cashapp/hermit.git",
			"https://x-access-token:****@github.com/cashapp/hermit.git"},
		{"TokenOnly", "https://ghp_secret@github.com/o/r.git", "https://****@github.com/o/r.git"},
		{"EmptyPassword", "https://user:@github.com/o/r.git", "https://user:****@github.com/o/r.git"},
		{"WithinCommandLine",
			"git clone -- https://x-access-token:ghp_secret@github.com/o/r.git /tmp/x failed",
			"git clone -- https://x-access-token:****@github.com/o/r.git /tmp/x failed"},
		{"MultipleURLs",
			"https://a:b@host/x https://c:d@host/y",
			"https://a:****@host/x https://c:****@host/y"},
		{"NonHTTPScheme", "ssh://git:secret@github.com/o/r.git", "ssh://git:****@github.com/o/r.git"},
		{"SCPStyleUnchanged", "git@github.com:cashapp/hermit.git", "git@github.com:cashapp/hermit.git"},
		{"EmailInMessageUnchanged", "contact user@example.com for access", "contact user@example.com for access"},
		{"PathAfterHostUnchanged", "https://github.com/o/r@v1.git", "https://github.com/o/r@v1.git"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, RedactCredentials(test.input))
		})
	}
}
