package sources

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/ui"
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
