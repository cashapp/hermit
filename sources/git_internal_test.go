package sources

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

func statWithModTime(t *testing.T, modTime time.Time) os.FileInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f")
	assert.NoError(t, os.WriteFile(path, nil, 0600))
	assert.NoError(t, os.Chtimes(path, modTime, modTime))
	info, err := os.Stat(path)
	assert.NoError(t, err)
	return info
}

func TestSyncedSince(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		info os.FileInfo
		want bool
	}{
		{"no directory", nil, false},
		{"synced well before", statWithModTime(t, now.Add(-2*time.Second)), false},
		{"synced exactly at instant", statWithModTime(t, now), true},
		{"synced within filesystem granularity slack", statWithModTime(t, now.Add(-500*time.Millisecond)), true},
		{"synced after instant", statWithModTime(t, now.Add(time.Second)), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, syncedSince(test.info, now))
		})
	}
}

// TestRemoveStaleScratchDirs verifies the age-gated cleanup sweep only
// touches Hermit's own scratch-directory naming conventions, and only once
// they're old enough that another (older, lock-unaware) Hermit process is
// unlikely to still be using them.
func TestRemoveStaleScratchDirs(t *testing.T) {
	dir := t.TempDir()
	finalDest := filepath.Join(dir, "abc123")
	assert.NoError(t, os.MkdirAll(finalDest, 0700))

	touch := func(name string, age time.Duration) {
		p := filepath.Join(dir, name)
		assert.NoError(t, os.MkdirAll(p, 0700))
		mt := time.Now().Add(-age)
		assert.NoError(t, os.Chtimes(p, mt, mt))
	}
	touch("abc123.tmp-old", 48*time.Hour) // stale clone scratch: removed
	touch("abc123.tmp-new", time.Minute)  // fresh clone scratch: may be in use, kept
	touch("abc123.old", 48*time.Hour)     // stale swap-aside: removed
	touch("abc123-legacy", 48*time.Hour)  // pre-lock-era clone scratch: removed
	touch("abc123.lock", 48*time.Hour)    // lock file: always kept, regardless of age
	touch("def456", 48*time.Hour)         // unrelated real source dir: untouched

	u, _ := ui.NewForTesting()
	removeStaleScratchDirs(u, dir, finalDest)

	assertExists := func(name string, want bool) {
		t.Helper()
		_, err := os.Stat(filepath.Join(dir, name))
		if want {
			assert.NoError(t, err, name)
		} else {
			assert.True(t, os.IsNotExist(err), name)
		}
	}
	assertExists("abc123", true)
	assertExists("abc123.tmp-new", true)
	assertExists("abc123.lock", true)
	assertExists("def456", true)
	assertExists("abc123.tmp-old", false)
	assertExists("abc123.old", false)
	assertExists("abc123-legacy", false)
}

// runGit runs a real "git" command in dir, failing the test on error. Used to
// build and update the upstream repo that syncGit clones/fetches from --
// separate from the util.CommandRunner under test, which is what syncGit
// itself uses to clone/fetch/checkout.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "git %v: %s", args, out)
}

// gitEnvIsolated points HOME, XDG_CONFIG_HOME and the global/system gitconfig
// locations at throwaway paths for the duration of t, so a hook, alias or
// setting in the machine running the test (eg. commit.gpgsign, a
// core.hooksPath) can't affect a test that only cares about plumbing
// commands.
func gitEnvIsolated(t *testing.T) {
	t.Helper()
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("XDG_CONFIG_HOME", empty)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(empty, "gitconfig-does-not-exist"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(empty, "gitconfig-does-not-exist"))
}

// TestSyncGitIncrementalUpdate exercises syncGit's incremental-update branch
// (taken once finalDest is already a clone) against a real "git" binary. This
// replaced a "--reference-if-able" clone that turned out to be a silent
// no-op against Hermit's own shallow (--depth=1) clones -- no existing test
// drove the actual clone/fetch/checkout mechanism at all, which is how it
// shipped broken. syncGit is called directly (rather than via GitSource.Sync)
// so this doesn't depend on the wall-clock/mtime-granularity slack in
// syncedSince's double-checked-locking skip.
//
// The upstream repo is addressed via a "file://" URL rather than a bare local
// path: git silently ignores "--depth" for a local-filesystem path ("--depth
// is ignored in local clones"), which would make finalDest never actually
// shallow and defeat the point of this test -- "file://" forces the real
// smart-transport, shallow-fetch code path, the same one used against a real
// remote like the default hermit-packages source.
func TestSyncGitIncrementalUpdate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	gitEnvIsolated(t)

	upstreamDir := t.TempDir()
	upstream := "file://" + upstreamDir
	runGit(t, upstreamDir, "init", "-q", "-b", "main")
	runGit(t, upstreamDir, "config", "user.email", "test@example.com")
	runGit(t, upstreamDir, "config", "user.name", "Test")
	assert.NoError(t, os.WriteFile(filepath.Join(upstreamDir, "first.hcl"), []byte("description = \"first\"\n"), 0600))
	runGit(t, upstreamDir, "add", "first.hcl")
	runGit(t, upstreamDir, "commit", "-q", "-m", "first")

	dir := t.TempDir()
	finalDest := filepath.Join(dir, "abc123")
	u, _ := ui.NewForTesting()
	runner := &util.RealCommandRunner{}

	// Initial clone: finalDest has no ".git" yet, so this takes the
	// fresh-clone branch.
	assert.NoError(t, syncGit(u.Task("test"), dir, upstream, finalDest, runner))
	first, err := os.ReadFile(filepath.Join(finalDest, "first.hcl"))
	assert.NoError(t, err)
	assert.Equal(t, "description = \"first\"\n", string(first))

	// Sync several more times, each adding a new commit upstream: finalDest
	// now has a ".git", so every one of these takes the incremental-update
	// branch. Repeating this (rather than syncing just once more) is what
	// actually exercises incrementalBranch's persistence -- a version of this
	// path that left finalDest with a detached HEAD passed a single-sync
	// version of this test, but silently degraded into a full-cost clone
	// starting from the second incremental sync.
	for i := 2; i <= 4; i++ {
		name := fmt.Sprintf("commit%d.hcl", i)
		content := fmt.Sprintf("description = \"commit %d\"\n", i)
		assert.NoError(t, os.WriteFile(filepath.Join(upstreamDir, name), []byte(content), 0600))
		runGit(t, upstreamDir, "add", name)
		runGit(t, upstreamDir, "commit", "-q", "-m", fmt.Sprintf("commit %d", i))

		assert.NoError(t, syncGit(u.Task("test"), dir, upstream, finalDest, runner))

		got, err := os.ReadFile(filepath.Join(finalDest, name))
		assert.NoError(t, err, "sync %d", i)
		assert.Equal(t, content, string(got), "sync %d", i)

		// incrementalBranch must persist as a real ref across syncs -- if
		// this is ever a detached HEAD instead, the *next* sync's local
		// clone of finalDest has no branch to send as a "have", and its
		// fetch silently regresses into transferring a full pack.
		branch, err := exec.Command("git", "-C", finalDest, "branch", "--show-current").CombinedOutput()
		assert.NoError(t, err, "sync %d: %s", i, branch)
		assert.Equal(t, incrementalBranch, strings.TrimSpace(string(branch)), "sync %d", i)
	}

	// Deleting a file upstream must propagate too: "checkout -B" replaces the
	// whole tree, it doesn't merge, so this would fail if the incremental
	// path ever left a stale copy of a removed file behind.
	runGit(t, upstreamDir, "rm", "-q", "first.hcl")
	runGit(t, upstreamDir, "commit", "-q", "-m", "remove first.hcl")
	assert.NoError(t, syncGit(u.Task("test"), dir, upstream, finalDest, runner))
	_, err = os.Stat(filepath.Join(finalDest, "first.hcl"))
	assert.True(t, os.IsNotExist(err), "first.hcl should have been removed by the incremental checkout")

	info, err := os.Stat(filepath.Join(finalDest, ".git"))
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	// No leftover clone/swap scratch directories.
	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(entries), "expected only finalDest, got %v", entries)
}

// TestSyncGitIncrementalUpdateFallsBackOnCorruptClone verifies that a
// corrupt/truncated finalDest ".git" (eg. from an interrupted earlier write,
// or a filesystem issue) doesn't wedge every future sync: syncGit should
// notice the incremental path failed and fall back to a fresh clone, the way
// a from-scratch sync always could, rather than leaving finalDest stuck with
// a warning on every subsequent command.
func TestSyncGitIncrementalUpdateFallsBackOnCorruptClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	gitEnvIsolated(t)

	upstreamDir := t.TempDir()
	upstream := "file://" + upstreamDir
	runGit(t, upstreamDir, "init", "-q", "-b", "main")
	runGit(t, upstreamDir, "config", "user.email", "test@example.com")
	runGit(t, upstreamDir, "config", "user.name", "Test")
	assert.NoError(t, os.WriteFile(filepath.Join(upstreamDir, "first.hcl"), []byte("description = \"first\"\n"), 0600))
	runGit(t, upstreamDir, "add", "first.hcl")
	runGit(t, upstreamDir, "commit", "-q", "-m", "first")

	dir := t.TempDir()
	finalDest := filepath.Join(dir, "abc123")
	// A finalDest with a ".git" directory that isn't actually a valid repo:
	// takes the incremental branch, and "git clone --no-checkout finalDest
	// dest" must fail against it.
	assert.NoError(t, os.MkdirAll(filepath.Join(finalDest, ".git"), 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(finalDest, "stale.hcl"), []byte("stale"), 0600))

	u, _ := ui.NewForTesting()
	runner := &util.RealCommandRunner{}
	assert.NoError(t, syncGit(u.Task("test"), dir, upstream, finalDest, runner))

	first, err := os.ReadFile(filepath.Join(finalDest, "first.hcl"))
	assert.NoError(t, err)
	assert.Equal(t, "description = \"first\"\n", string(first))
	_, err = os.Stat(filepath.Join(finalDest, "stale.hcl"))
	assert.True(t, os.IsNotExist(err), "stale content from the corrupt clone should not survive")
}

// TestHoldSourceLockChildProcess is not a real test: it's a worker spawned by
// TestSyncLockTimeoutFallsBackToExistingCopy (sources_test package) via
// re-exec, guarded by an env var so it no-ops under a normal test run. It
// holds the sync lock for a directory from a genuinely separate process,
// which is required to exercise lock contention: util/flock is deliberately
// re-entrant per-PID, so a single process can never observe its own lock as
// held by someone else.
func TestHoldSourceLockChildProcess(t *testing.T) {
	if os.Getenv("HERMIT_TEST_CHILD") == "" {
		t.Skip("only runs as a spawned child of TestSyncLockTimeoutFallsBackToExistingCopy")
	}
	path := os.Getenv("HERMIT_TEST_LOCK_PATH")
	hold, err := time.ParseDuration(os.Getenv("HERMIT_TEST_LOCK_HOLD"))
	if err != nil {
		t.Fatalf("bad hold duration: %s", err)
	}
	u, _ := ui.NewForTesting()
	release, err := acquireSyncLock(u, path, DefaultLockTimeout, "test lock holder")
	if err != nil {
		t.Fatalf("failed to acquire lock: %s", err)
	}
	if readyFile := os.Getenv("HERMIT_TEST_LOCK_READY_FILE"); readyFile != "" {
		if err := os.WriteFile(readyFile, nil, 0600); err != nil {
			t.Fatalf("failed to signal lock held: %s", err)
		}
	}
	time.Sleep(hold)
	if err := release(); err != nil {
		t.Fatalf("failed to release lock: %s", err)
	}
}
