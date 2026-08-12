package util_test

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/util"
)

func TestValidateGitURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		fails bool
	}{
		{"HTTPS", "https://github.com/cashapp/hermit-packages.git", false},
		{"HTTP", "http://example.com/repo.git", false},
		{"SSH", "ssh://git@github.com/owner/repo.git", false},
		{"Git", "git://example.com/repo.git", false},
		{"File", "file:///tmp/packages/.git", false},
		{"UppercaseScheme", "HTTPS://github.com/owner/repo.git", false},
		{"SCPLike", "git@github.com:owner/repo.git", false},
		{"AbsolutePath", "/tmp/packages/.git", false},
		{"RelativePath", "../packages/.git", false},
		{"ColonsInPath", "https://example.com/a::b.git", false},
		{"RemoteHelper", "zzq::x.git", true},
		{"ExtRemoteHelper", "ext::sh -c whoami", true},
		{"UnknownScheme", "zzq://example.com/repo.git", true},
		{"LeadingDash", "--upload-pack=touch /tmp/pwned", true},
		{"SchemeOnly", "://example.com/repo.git", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := util.ValidateGitURL(test.url)
			if test.fails {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGitArgsPinsTransportPolicy(t *testing.T) {
	args := util.GitArgs("clone", "--", "https://example.com/repo.git")
	assert.Equal(t, []string{
		"git",
		"-c", "protocol.allow=never",
		"-c", "protocol.file.allow=always",
		"-c", "protocol.git.allow=always",
		"-c", "protocol.http.allow=always",
		"-c", "protocol.https.allow=always",
		"-c", "protocol.ssh.allow=always",
		"clone", "--", "https://example.com/repo.git",
	}, args)
}
