package cache

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestGitParseRepo(t *testing.T) {
	repo, tag, err := parseGitURL("org-49461806@github.com:squareup/orc.git")
	assert.NoError(t, err)
	assert.Equal(t, "org-49461806@github.com:squareup/orc.git", repo)
	assert.Equal(t, "", tag)
	repo, tag, err = parseGitURL("org-49461806@github.com:squareup/orc.git#v1.2.3")
	assert.NoError(t, err)
	assert.Equal(t, "org-49461806@github.com:squareup/orc.git", repo)
	assert.Equal(t, "v1.2.3", tag)
}

func TestParseGitURLArgumentInjection(t *testing.T) {
	tests := []struct {
		url         string
		expectError bool
	}{
		{"--upload-pack=sh -c 'echo OWNED' #file:///tmp/repo/.git", true},
		{"--config core.sshCommand='touch /tmp/pwned' git@github.com:fake/repo.git", true},
		{"-v https://github.com/user/repo.git", true},
		{"https://github.com/cashapp/hermit.git#v1.0.0", false},
		{"git@github.com:cashapp/hermit.git#main", false},
		{"file:///path/to/repo.git", false},
	}

	for _, tt := range tests {
		_, _, err := parseGitURL(tt.url)
		if tt.expectError {
			assert.Error(t, err, "Should reject: "+tt.url)
		} else {
			assert.NoError(t, err, "Should accept: "+tt.url)
		}
	}
}

// TestGitSourcePreventRCE verifies all git operations reject malicious URLs.
func TestGitSourcePreventRCE(t *testing.T) {
	tmpDir := t.TempDir()
	pwnedFile := filepath.Join(tmpDir, "pwned")
	maliciousURL := "--upload-pack=sh -c 'echo OWNED > " + pwnedFile + "' #file://" + tmpDir + "/.git"

	src := &gitSource{URL: maliciousURL}
	err := src.Validate()
	assert.Error(t, err)

	cache := &Cache{root: tmpDir}
	_, _, _, err = src.Download(nil, cache, "test")
	assert.Error(t, err)

	_, err = src.ETag(nil)
	assert.Error(t, err)

	_, fileErr := os.Stat(pwnedFile)
	assert.True(t, os.IsNotExist(fileErr))
}

// TestGitSourceRCEAttempt simulates a real attack with an actual git repository.
func TestGitSourceRCEAttempt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(repoDir, 0750))
	assert.NoError(t, exec.Command("git", "init", repoDir).Run())

	pwnedFile := filepath.Join(tmpDir, "pwned")
	payload := "--upload-pack=sh -c 'echo OWNED > " + pwnedFile + "' #file://" + repoDir + "/.git"

	src := &gitSource{URL: payload}
	err := src.Validate()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid git URL")

	_, fileErr := os.Stat(pwnedFile)
	if !os.IsNotExist(fileErr) {
		t.Fatal("SECURITY FAILURE: RCE was NOT prevented!")
	}
}

func TestGitURLParsing(t *testing.T) {
	tests := []struct {
		url  string
		repo string
		tag  string
	}{
		{"org-49461806@github.com:squareup/orc.git", "org-49461806@github.com:squareup/orc.git", ""},
		{"org-49461806@github.com:squareup/orc.git#v1.2.3", "org-49461806@github.com:squareup/orc.git", "v1.2.3"},
		{"https://github.com/cashapp/hermit.git#main", "https://github.com/cashapp/hermit.git", "main"},
		{"file:///home/user/repo.git#develop", "file:///home/user/repo.git", "develop"},
	}

	for _, tt := range tests {
		repo, tag, err := parseGitURL(tt.url)
		assert.NoError(t, err)
		assert.Equal(t, tt.repo, repo)
		assert.Equal(t, tt.tag, tag)
	}
}
