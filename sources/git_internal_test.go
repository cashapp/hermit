package sources

import (
	"os"
	"os/exec"
	"path/filepath"
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

// TestSyncGitIncrementalUpdate exercises syncGit's incremental-update branch
// (taken once finalDest is already a clone) against a real "git" binary. This
// replaced a "--reference-if-able" clone that turned out to be a silent
// no-op against Hermit's own shallow (--depth=1) clones -- no existing test
// drove the actual clone/fetch/checkout mechanism at all, which is how it
// shipped broken. syncGit is called directly (rather than via GitSource.Sync)
// so this doesn't depend on the wall-clock/mtime-granularity slack in
// syncedSince's double-checked-locking skip.
func TestSyncGitIncrementalUpdate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	upstream := t.TempDir()
	runGit(t, upstream, "init", "-q", "-b", "main")
	runGit(t, upstream, "config", "user.email", "test@example.com")
	runGit(t, upstream, "config", "user.name", "Test")
	assert.NoError(t, os.WriteFile(filepath.Join(upstream, "first.hcl"), []byte("description = \"first\"\n"), 0600))
	runGit(t, upstream, "add", "first.hcl")
	runGit(t, upstream, "commit", "-q", "-m", "first")

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

	// Add a second commit upstream, then sync again: finalDest now has a
	// ".git", so this must take the incremental-update branch.
	assert.NoError(t, os.WriteFile(filepath.Join(upstream, "second.hcl"), []byte("description = \"second\"\n"), 0600))
	runGit(t, upstream, "add", "second.hcl")
	runGit(t, upstream, "commit", "-q", "-m", "second")

	assert.NoError(t, syncGit(u.Task("test"), dir, upstream, finalDest, runner))

	// The new commit's content must be present...
	second, err := os.ReadFile(filepath.Join(finalDest, "second.hcl"))
	assert.NoError(t, err)
	assert.Equal(t, "description = \"second\"\n", string(second))
	// ...and finalDest must still be a valid, checked-out git repo, not left
	// mid-checkout by "git clone --no-checkout".
	first, err = os.ReadFile(filepath.Join(finalDest, "first.hcl"))
	assert.NoError(t, err)
	assert.Equal(t, "description = \"first\"\n", string(first))
	info, err := os.Stat(filepath.Join(finalDest, ".git"))
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	// No leftover clone/swap scratch directories.
	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(entries), "expected only finalDest, got %v", entries)
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
