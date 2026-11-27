package sources_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

type FailingGit struct {
	err error
}

func (f *FailingGit) RunInDir(_ *ui.Task, _ string, _ ...redact.Value) error {
	return f.err
}

func (f *FailingGit) CaptureInDir(_ ui.Logger, _ string, _ ...redact.Value) ([]byte, error) {
	return nil, f.err
}

type CapturingGit struct {
	commands [][]string
}

func (c *CapturingGit) RunInDir(_ *ui.Task, _ string, args ...redact.Value) error {
	c.commands = append(c.commands, redact.Reveal(args))
	return nil
}

func (c *CapturingGit) CaptureInDir(_ ui.Logger, _ string, args ...redact.Value) ([]byte, error) {
	c.commands = append(c.commands, redact.Reveal(args))
	return nil, nil
}

func TestGitDoesNotRemoveSourceAfterSyncFailure(t *testing.T) {
	git := &FailingGit{}
	sourceDir := t.TempDir()
	source := sources.NewGitSource(redact.URL("git://test"), sourceDir, git)

	// Create the initial directory for sources by successfully syncing
	u, _ := ui.NewForTesting()
	_, err := source.Sync(u, true)
	assert.NoError(t, err)
	checkouts := checkoutDirs(t, sourceDir)
	assert.Equal(t, 1, len(checkouts))

	// Fail the sync
	git.err = errors.New("failing git fails")
	_, err = source.Sync(u, true)

	// no error as it was not an initial clone
	assert.NoError(t, err)

	// the directory should still be in place after git failed to update
	assert.Equal(t, checkouts, checkoutDirs(t, sourceDir))
}

// checkoutDirs returns the source checkouts in sourceDir, leaving out their sync lock files.
func checkoutDirs(t *testing.T, sourceDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(sourceDir)
	assert.NoError(t, err)
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	return dirs
}

const (
	syncHelperDirEnv  = "HERMIT_TEST_SYNC_HELPER_DIR"
	syncHelperRepoEnv = "HERMIT_TEST_SYNC_HELPER_REPO"
)

// Concurrent first uses each force a sync after failing to find a package.
// Unserialised, those syncs replaced the checkout while other processes read
// manifests from it, which failed with "unknown package".
func TestConcurrentGitSyncsKeepManifestsReadable(t *testing.T) {
	git, err := exec.LookPath("git")
	assert.NoError(t, err)
	repo := filepath.Join(t.TempDir(), "manifests.git")
	assert.NoError(t, os.Mkdir(repo, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(repo, "test.hcl"), []byte(`description = "test"`), 0600))
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "test.hcl"},
		{"-c", "user.name=Hermit", "-c", "user.email=hermit@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "initial"},
	} {
		output, err := exec.Command(git, append([]string{"-C", repo}, args...)...).CombinedOutput()
		assert.NoError(t, err, "%s", output)
	}

	for range 3 {
		sourceDir := t.TempDir()
		helpers := make([]*exec.Cmd, 8)
		outputs := make([]*strings.Builder, len(helpers))
		for i := range helpers {
			outputs[i] = &strings.Builder{}
			helpers[i] = exec.Command(os.Args[0], "-test.run=^TestGitSyncHelperProcess$")
			helpers[i].Env = append(os.Environ(), syncHelperDirEnv+"="+sourceDir, syncHelperRepoEnv+"=file://"+repo)
			helpers[i].Stdout = outputs[i]
			helpers[i].Stderr = outputs[i]
			assert.NoError(t, helpers[i].Start())
		}
		for i, helper := range helpers {
			assert.NoError(t, helper.Wait(), "%s", outputs[i])
		}
	}
}

// TestGitSyncHelperProcess runs as a separate process for
// TestConcurrentGitSyncsKeepManifestsReadable. It syncs the source, then keeps
// reading a manifest while other processes sync.
func TestGitSyncHelperProcess(t *testing.T) {
	sourceDir := os.Getenv(syncHelperDirEnv)
	if sourceDir == "" {
		t.Skip("only runs as a helper process")
	}
	source := sources.NewGitSource(redact.URL(os.Getenv(syncHelperRepoEnv)), sourceDir, &util.RealCommandRunner{})
	u, _ := ui.NewForTesting()
	_, err := source.Sync(u, true)
	assert.NoError(t, err)
	for range 200 {
		_, err := fs.ReadFile(source.Bundle(), "test.hcl")
		assert.NoError(t, err)
		time.Sleep(time.Millisecond)
	}
}

func TestGitSyncUsesCredentialsButDisplaysRedactedURI(t *testing.T) {
	git := &CapturingGit{}
	source := sources.NewGitSource(redact.URL("https://x-access-token:sekret@github.com/owner/repo.git"), t.TempDir(), git)
	assert.Equal(t, "https://github.com/owner/repo.git", source.URI())

	u, buf := ui.NewForTesting()
	_, err := source.Sync(u, true)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(git.commands))
	assert.Contains(t, strings.Join(git.commands[0], " "), "https://x-access-token:sekret@github.com/owner/repo.git")
	assert.NotContains(t, buf.String(), "sekret")
}

func TestGitSyncDoesNotLeakURLCredentials(t *testing.T) {
	u, buf := ui.NewForTesting()
	source := sources.NewGitSource(redact.URL("https://x-access-token:sekret@127.0.0.1:1/owner/repo.git"), t.TempDir(), &util.RealCommandRunner{})

	_, err := source.Sync(u, true)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")
	assert.Contains(t, err.Error(), "https://127.0.0.1:1/owner/repo.git")
	assert.NotContains(t, buf.String(), "sekret")
}

// gitTestEnv points git at a test-scoped global config, so a test does not
// depend on, or change, the user's own git configuration.
func gitTestEnv(t *testing.T, extraConfig string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found")
	}
	config := filepath.Join(t.TempDir(), "gitconfig")
	assert.NoError(t, os.WriteFile(config, []byte(`[user]
	name = Test
	email = test@example.com
[commit]
	gpgsign = false
[init]
	defaultBranch = main
`+extraConfig), 0600))
	t.Setenv("GIT_CONFIG_GLOBAL", config)
}

// mustGit runs git in dir, ignoring any repository the test environment
// points at.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = util.WithoutGitLocalEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, repo, name, content string) string {
	t.Helper()
	assert.NoError(t, os.WriteFile(filepath.Join(repo, name), []byte(content), 0600))
	mustGit(t, repo, "add", name)
	mustGit(t, repo, "commit", "-m", name)
	return mustGit(t, repo, "rev-parse", "HEAD")
}

// newUpstream creates a repository with one commit to act as a package source.
func newUpstream(t *testing.T) string {
	t.Helper()
	upstream := filepath.Join(t.TempDir(), "upstream")
	mustGit(t, t.TempDir(), "init", upstream)
	commitFile(t, upstream, "first.hcl", "first")
	return upstream
}

func syncSource(t *testing.T, source *sources.GitSource) {
	t.Helper()
	u, _ := ui.NewForTesting()
	_, err := source.Sync(u, true)
	assert.NoError(t, err)
}

// When Hermit runs inside a git hook, git exports variables such as GIT_DIR
// and GIT_INDEX_FILE that point at the repository running the hook (in a
// linked worktree, for example). Syncing a source must act on the source's
// cache, never on that repository, even with pull.rebase set.
func TestGitSyncIgnoresCallerRepositoryEnvironment(t *testing.T) {
	gitTestEnv(t, "[pull]\n\trebase = true\n")
	upstream := newUpstream(t)
	sourceDir := t.TempDir()
	source := sources.NewGitSource(redact.URL("file://"+upstream), sourceDir, &util.RealCommandRunner{})
	syncSource(t, source)
	upstreamHead := commitFile(t, upstream, "second.hcl", "second")

	// The caller's branch has a local commit and tracks a remote that has
	// moved on, so a pull run against it would rebase that branch.
	callerRemote := filepath.Join(t.TempDir(), "caller-remote")
	mustGit(t, t.TempDir(), "init", callerRemote)
	commitFile(t, callerRemote, "caller.txt", "one")
	caller := filepath.Join(t.TempDir(), "caller")
	mustGit(t, t.TempDir(), "clone", "--quiet", callerRemote, caller)
	commitFile(t, callerRemote, "remote.txt", "two")
	commitFile(t, caller, "local.txt", "local")
	callerHead := mustGit(t, caller, "rev-parse", "HEAD")
	callerIndex, err := os.ReadFile(filepath.Join(caller, ".git", "index"))
	assert.NoError(t, err)

	t.Setenv("GIT_DIR", filepath.Join(caller, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(caller, ".git", "index"))
	syncSource(t, source)

	assert.Equal(t, callerHead, mustGit(t, caller, "rev-parse", "HEAD"))
	assert.Equal(t, "", mustGit(t, caller, "status", "--porcelain"))
	index, err := os.ReadFile(filepath.Join(caller, ".git", "index"))
	assert.NoError(t, err)
	assert.Equal(t, callerIndex, index)

	cache := filepath.Join(sourceDir, util.Hash("file://"+upstream))
	assert.Equal(t, upstreamHead, mustGit(t, cache, "rev-parse", "HEAD"))
}

// A refresh must leave the cache exactly at the source's head whatever the
// user's pull settings, both for a normal update and after the source's
// history was rewritten.
func TestGitSyncIgnoresPullConfig(t *testing.T) {
	for _, config := range []string{
		"[pull]\n\trebase = true\n",
		"[pull]\n\tff = false\n",
	} {
		for _, rewrite := range []bool{false, true} {
			name := strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(config, "[pull]\n\t")), " ", "")
			if rewrite {
				name += "/rewritten"
			}
			t.Run(name, func(t *testing.T) {
				gitTestEnv(t, config)
				upstream := newUpstream(t)
				sourceDir := t.TempDir()
				source := sources.NewGitSource(redact.URL("file://"+upstream), sourceDir, &util.RealCommandRunner{})
				syncSource(t, source)

				if rewrite {
					// Replace the source's history with an unrelated commit.
					mustGit(t, upstream, "checkout", "--quiet", "--orphan", "rewritten")
					mustGit(t, upstream, "rm", "--quiet", "-rf", ".")
					commitFile(t, upstream, "rewritten.hcl", "rewritten")
					mustGit(t, upstream, "branch", "-M", "main")
				} else {
					commitFile(t, upstream, "second.hcl", "second")
				}
				upstreamHead := mustGit(t, upstream, "rev-parse", "HEAD")

				syncSource(t, source)

				cache := filepath.Join(sourceDir, util.Hash("file://"+upstream))
				assert.Equal(t, upstreamHead, mustGit(t, cache, "rev-parse", "HEAD"))
			})
		}
	}
}
