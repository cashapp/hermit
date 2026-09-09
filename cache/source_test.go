package cache

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

func TestGitParseRepo(t *testing.T) {
	repo, tag, err := parseGitURL("org-49461806@github.com:squareup/orc.git")
	assert.NoError(t, err)
	assert.Equal(t, redact.URL("org-49461806@github.com:squareup/orc.git"), repo)
	assert.Equal(t, "", tag)
	repo, tag, err = parseGitURL("org-49461806@github.com:squareup/orc.git#v1.2.3")
	assert.NoError(t, err)
	assert.Equal(t, redact.URL("org-49461806@github.com:squareup/orc.git"), repo)
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
		{"zzq::x.git", true},
		{"ext::sh -c 'touch /tmp/pwned'", true},
		{"zzq://example.com/repo.git", true},
		{"https://github.com/cashapp/hermit.git#v1.0.0", false},
		{"git@github.com:cashapp/hermit.git#main", false},
		{"file:///path/to/repo.git", false},
	}

	for _, tt := range tests {
		_, _, err := parseGitURL(redact.URL(tt.url))
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

	src := &gitSource{URL: redact.URL(maliciousURL), runner: &util.RealCommandRunner{}}
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

	src := &gitSource{URL: redact.URL(payload), runner: &util.RealCommandRunner{}}
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
		repo, tag, err := parseGitURL(redact.URL(tt.url))
		assert.NoError(t, err)
		assert.Equal(t, redact.URL(tt.repo), repo)
		assert.Equal(t, tt.tag, tag)
	}
}

type capturingRunner struct {
	commands [][]string
	output   []byte
}

func (c *capturingRunner) RunInDir(_ *ui.Task, _ string, args ...redact.Value) error {
	c.commands = append(c.commands, redact.Reveal(args))
	return nil
}

func (c *capturingRunner) CaptureInDir(_ ui.Logger, _ string, args ...redact.Value) ([]byte, error) {
	c.commands = append(c.commands, redact.Reveal(args))
	return c.output, nil
}

func TestGitSourceUsesCredentialsButRedactsDisplay(t *testing.T) {
	runner := &capturingRunner{output: []byte("abc123\n")}
	src := &gitSource{URL: redact.URL("https://x-access-token:sekret@github.com/owner/repo.git#v1"), runner: runner}

	u, buf := ui.NewForTesting()
	task := u.Task("test")
	_, etag, _, err := src.Download(task, &Cache{root: t.TempDir()}, "checksum")
	assert.NoError(t, err)
	assert.Equal(t, "abc123", etag)

	clone := strings.Join(runner.commands[0], " ")
	assert.Contains(t, clone, "https://x-access-token:sekret@github.com/owner/repo.git")
	assert.Contains(t, clone, "--branch=v1")
	assert.NotContains(t, buf.String(), "sekret")
}

func TestHTTPSourceDoesNotLeakURLCredentials(t *testing.T) {
	u, buf := ui.NewForTesting()
	task := u.Task("test")
	source := HTTPSource(http.DefaultClient, "https://sekret-token@127.0.0.1:1/owner/file.tar.gz")

	_, _, _, err := source.Download(task, &Cache{root: t.TempDir()}, "checksum")
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret-token")
	assert.Contains(t, err.Error(), "https://127.0.0.1:1/owner/file.tar.gz")

	_, err = source.ETag(task)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret-token")

	err = source.Validate()
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret-token")

	assert.NotContains(t, buf.String(), "sekret-token")
}

func TestGitSourceDoesNotLeakURLCredentials(t *testing.T) {
	u, buf := ui.NewForTesting()
	task := u.Task("test")
	src := &gitSource{
		URL:    redact.URL("https://x-access-token:sekret@127.0.0.1:1/owner/repo.git"),
		runner: &util.RealCommandRunner{},
	}

	_, _, _, err := src.Download(task, &Cache{root: t.TempDir()}, "checksum")
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")

	_, err = src.ETag(task)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")
	assert.Contains(t, err.Error(), "https://127.0.0.1:1/owner/repo.git")

	err = src.Validate()
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")

	assert.NotContains(t, buf.String(), "sekret")
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

	src := &gitSource{URL: redact.URL("file://" + repoDir + "#" + pinned), runner: &util.RealCommandRunner{}}
	log, _ := ui.NewForTesting()

	// The remote advertises no branch or tag by this name, so it is treated
	// as a pinned commit and is its own ETag.
	etag, err := src.ETag(log.Task("test"))
	assert.NoError(t, err)
	assert.Equal(t, pinned, etag)

	assert.NoError(t, src.Validate())

	cacheRoot := filepath.Join(tmpDir, "cache")
	assert.NoError(t, os.MkdirAll(cacheRoot, 0750))
	cache := &Cache{root: cacheRoot}
	dir, etag, _, err := src.Download(log.Task("test"), cache, "checksum")
	assert.NoError(t, err)
	assert.Equal(t, pinned, etag)
	content, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "first", string(content))

	// A branch whose name is exactly a full-hex string must still resolve as
	// a branch, not be reinterpreted as a commit object ID.
	hexBranch := strings.Repeat("a", 40)
	mustGit("branch", hexBranch, "HEAD")
	branchSrc := &gitSource{URL: redact.URL("file://" + repoDir + "#" + hexBranch), runner: &util.RealCommandRunner{}}

	etag, err = branchSrc.ETag(log.Task("test"))
	assert.NoError(t, err)
	assert.Equal(t, mustGit("rev-parse", "HEAD"), etag)

	dir, etag, _, err = branchSrc.Download(log.Task("test"), cache, "checksum2")
	assert.NoError(t, err)
	assert.Equal(t, mustGit("rev-parse", "HEAD"), etag)
	content, err = os.ReadFile(filepath.Join(dir, "file.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "second", string(content))
}

func TestGitCommitSourceUsesProtectedTransport(t *testing.T) {
	runner := &capturingRunner{}
	repo := redact.URL("https://x-access-token:sekret@github.com/owner/repo.git")
	sha := strings.Repeat("a", 40)
	src := &gitSource{URL: redact.URL(repo.Reveal() + "#" + sha), runner: runner}
	log, _ := ui.NewForTesting()
	_, _, _, err := src.Download(log.Task("test"), &Cache{root: t.TempDir()}, "checksum")
	assert.NoError(t, err)
	assert.Equal(t, append(util.GitArgs("ls-remote", "--"), repo.Reveal(), "refs/heads/"+sha, "refs/tags/"+sha), runner.commands[0])
	assert.Equal(t, append(util.GitArgs("fetch", "--depth=1", "--"), repo.Reveal(), sha), runner.commands[2])
}

func TestGitCommitSourceDoesNotLeakURLCredentials(t *testing.T) {
	log, buf := ui.NewForTesting()
	task := log.Task("test")
	repo := redact.URL("https://x-access-token:sekret@127.0.0.1:1/owner/repo.git")
	sha := strings.Repeat("a", 40)
	src := &gitSource{URL: redact.URL(repo.Reveal() + "#" + sha), runner: &util.RealCommandRunner{}}
	_, _, _, err := src.Download(task, &Cache{root: t.TempDir()}, "checksum")
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")
	_, err = src.ETag(task)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")
	err = src.checkoutGitCommit(task, t.TempDir(), repo, sha, filepath.Join(t.TempDir(), "checkout"))
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")
	assert.NotContains(t, buf.String(), "sekret")
}
