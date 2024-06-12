package util

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/ui"
)

// Test that a lock eventually times out if the lock has been opened by someone else
func TestFileLockTimeout(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "lock")
	logger1, _ := ui.NewForTesting()
	logger2, logger2buf := ui.NewForTesting()

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	lock := NewLock(file, 5*time.Millisecond)
	err1 := lock.Acquire(timeoutCtx, logger1)
	assert.NoError(t, err1)
	defer lock.Release(logger1)

	lock2 := NewLock(file, 10*time.Millisecond)
	err2 := lock2.Acquire(timeoutCtx, logger2)

	assert.Equal(t, strings.HasPrefix(err2.Error(), "timeout while waiting for the lock"), true)

	assert.Contains(t, logger2buf.String(), "Waiting for a lock at "+file)
}

// Test that releasing a lock allows others to lock it again
func TestFileLockRelease(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "lock")
	logger1, logger1buf := ui.NewForTesting()
	logger2, logger2buf := ui.NewForTesting()

	lock1 := NewLock(file, 5*time.Millisecond)
	err1 := lock1.Acquire(context.Background(), logger1)
	assert.NoError(t, err1)
	lock1.Release(logger1)

	lock2 := NewLock(file, 5*time.Millisecond)
	err2 := lock2.Acquire(context.Background(), logger2)
	assert.NoError(t, err2)
	lock2.Release(logger2)

	assert.Zero(t, logger1buf.String())
	assert.Zero(t, logger2buf.String())
}

// Test that releasing a lock allows other waiting locks to proceed
func TestFileLockProceed(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "lock")
	logger1, logger1buf := ui.NewForTesting()
	logger2, logger2buf := ui.NewForTesting()

	lock1 := NewLock(file, 5*time.Millisecond)
	err1 := lock1.Acquire(context.Background(), logger1)
	timer := time.NewTimer(50 * time.Millisecond)
	go func() {
		for {
			select {
			case <-timer.C:
				lock1.Release(logger1)
				timer.Stop()
			}
		}
	}()
	assert.NoError(t, err1)

	lock2 := NewLock(file, 5*time.Millisecond)
	err2 := lock2.Acquire(context.Background(), logger2)
	assert.NoError(t, err2)
	lock2.Release(logger2)

	assert.Zero(t, logger1buf.String())
	assert.Contains(t, logger2buf.String(), "Waiting for a lock at "+file)
}
