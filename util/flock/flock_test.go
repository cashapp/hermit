package flock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

// lockExternally holds an flock on path through its own open file description, as another process would, and
// records pid as the holder.
func lockExternally(t *testing.T, path string, pid int) (unlock func()) {
	t.Helper()
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC, 0600)
	assert.NoError(t, err)
	assert.NoError(t, unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB))
	payload, err := json.Marshal(pidFile{PID: pid, Message: "external"})
	assert.NoError(t, err)
	assert.NoError(t, unix.Ftruncate(fd, 0))
	_, err = unix.Write(fd, payload)
	assert.NoError(t, err)
	closed := false
	unlock = func() {
		if !closed {
			closed = true
			_ = unix.Close(fd)
		}
	}
	t.Cleanup(unlock)
	return unlock
}

// lockedExternally reports whether another open file description is blocked from locking path.
func lockedExternally(t *testing.T, path string) bool {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0600)
	assert.NoError(t, err)
	defer unix.Close(fd) //nolint:errcheck
	return unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil
}

func TestFlockHeldByOtherProcess(t *testing.T) {
	lockfile := filepath.Join(t.TempDir(), "lock")
	lockExternally(t, lockfile, 123)

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*200)
	defer cancel()
	_, err := Acquire(ctx, lockfile, "test")
	assert.IsError(t, err, ErrTimeout)
	assert.Contains(t, err.Error(), "123")
	assert.Contains(t, err.Error(), "external")
}

// Processes in different PID namespaces can share a PID, so a matching PID must not be treated as ownership.
func TestFlockHeldByOtherProcessWithSamePID(t *testing.T) {
	lockfile := filepath.Join(t.TempDir(), "lock")
	lockExternally(t, lockfile, os.Getpid())

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*200)
	defer cancel()
	_, err := Acquire(ctx, lockfile, "test")
	assert.IsError(t, err, ErrTimeout)
}

func TestRecursiveFlock(t *testing.T) {
	lockfile := filepath.Join(t.TempDir(), "lock")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	releaseOuter, err := Acquire(ctx, lockfile, "outer")
	assert.NoError(t, err)

	releaseInner, err := Acquire(ctx, lockfile, "inner")
	assert.NoError(t, err)

	pidBytes, err := os.ReadFile(lockfile)
	assert.NoError(t, err)
	pid := pidFile{}
	assert.NoError(t, json.Unmarshal(pidBytes, &pid))
	assert.Equal(t, pidFile{PID: os.Getpid(), Message: "outer"}, pid)

	assert.NoError(t, releaseInner())
	// A repeated release must not drop the outer holder's hold.
	assert.NoError(t, releaseInner())
	assert.True(t, lockedExternally(t, lockfile), "lock released while the outer holder still holds it")

	assert.NoError(t, releaseOuter())
	assert.False(t, lockedExternally(t, lockfile), "lock still held after every holder released it")
	_, err = os.Stat(lockfile)
	assert.NoError(t, err, "lock file must not be deleted")
}

// Test that a waiting Acquire succeeds after the other process releases the lock.
func TestContinueFlock(t *testing.T) {
	lockfile := filepath.Join(t.TempDir(), "lock")
	unlock := lockExternally(t, lockfile, os.Getpid())

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	wg, ctx := errgroup.WithContext(ctx)
	wg.Go(func() error {
		release, err := Acquire(ctx, lockfile, "test")
		if err != nil {
			return err
		}
		return release()
	})
	time.Sleep(time.Millisecond * 100)
	unlock()

	assert.NoError(t, wg.Wait())
}
