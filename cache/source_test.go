package cache

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/ui"
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

func TestIsFullGitSHA(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"6bccbcae2934bdd10ede93d493ee1eeeef5f24e2", true},
		{strings.Repeat("a", 64), true},
		{"", false},
		{"main", false},
		{"v1.2.3", false},
		// Abbreviated SHAs are indistinguishable from branch names.
		{"6bccbca", false},
		{strings.Repeat("a", 39), false},
		{strings.Repeat("a", 41), false},
		// Git prints SHAs in lowercase.
		{strings.Repeat("A", 40), false},
		{strings.Repeat("g", 40), false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isFullGitSHA(tt.ref), tt.ref)
	}
}

// TestGitSourceCommitSHAPinning verifies that a git source can be pinned to a
// full commit SHA rather than a branch or tag.
func TestGitSourceCommitSHAPinning(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(repoDir, 0750))
	mustGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	mustGit("init")
	mustGit("config", "user.email", "test@example.com")
	mustGit("config", "user.name", "Test")
	// Fetching an unadvertised commit from a local repository requires this;
	// hosting services such as GitHub and GitLab allow it by default.
	mustGit("config", "uploadpack.allowReachableSHA1InWant", "true")
	assert.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("first"), 0600))
	mustGit("add", "file.txt")
	mustGit("commit", "-m", "first")
	pinned := mustGit("rev-parse", "HEAD")
	assert.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("second"), 0600))
	mustGit("add", "file.txt")
	mustGit("commit", "-m", "second")

	src := &gitSource{URL: "file://" + repoDir + "#" + pinned}

	// The ETag of a pinned commit is the commit itself, with no remote call.
	etag, err := src.ETag(nil)
	assert.NoError(t, err)
	assert.Equal(t, pinned, etag)

	assert.NoError(t, src.Validate())

	cacheRoot := filepath.Join(tmpDir, "cache")
	assert.NoError(t, os.MkdirAll(cacheRoot, 0750))
	cache := &Cache{root: cacheRoot}
	log, _ := ui.NewForTesting()
	dir, etag, _, err := src.Download(log.Task("test"), cache, "checksum")
	assert.NoError(t, err)
	assert.Equal(t, pinned, etag)
	content, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "first", string(content))
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
