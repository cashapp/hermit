package sources

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestSource(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		expected string
	}{
		{"NoCredentials", "https://github.com/cashapp/hermit.git", "https://github.com/cashapp/hermit.git"},
		{"UserAndPassword", "https://x-access-token:ghp_secret@github.com/o/r.git", "https://x-access-token:****@github.com/o/r.git"},
		{"TokenOnly", "https://ghp_secret@github.com/o/r.git", "https://****@github.com/o/r.git"},
		{"EmptyPassword", "https://user:@github.com/o/r.git", "https://user:****@github.com/o/r.git"},
		{"UnescapedAtInPassword", "https://user:@secret@host/repo.git", "https://user:****@host/repo.git"},
		{"MultipleURLs", "https://a:b@host/x https://c:d@host/y", "https://a:****@host/x https://c:****@host/y"},
		{"NonHTTPPassword", "ssh://git:secret@github.com/o/r.git", "ssh://git:****@github.com/o/r.git"},
		{"SSHUsername", "ssh://git@github.com/o/r.git", "ssh://git@github.com/o/r.git"},
		{"SCPStyle", "git@github.com:cashapp/hermit.git", "git@github.com:cashapp/hermit.git"},
		{"AtInPath", "https://github.com/o/r@v1.git", "https://github.com/o/r@v1.git"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := NewSource(test.input)
			assert.Equal(t, test.input, source.Get())
			assert.Equal(t, test.expected, source.String())
		})
	}
}
