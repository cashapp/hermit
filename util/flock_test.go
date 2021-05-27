package util

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cashapp/hermit/ui"
)

// Test that a lock eventually times out if the lock has been opened by someone else
func TestFileLockTimeout(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "lock")
	logger1, _ := ui.NewForTesting()
	logger2, logger2buf := ui.NewForTesting()

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	lock := NewLock(file, 5*time.Millisecond)
	err1 := lock.Acquire(timeoutCtx, logger1)
	require.NoError(t, err1)
	defer lock.Release(logger1)

	lock2 := NewLock(file, 10*time.Millisecond)
	err2 := lock2.Acquire(timeoutCtx, logger2)
	require.Equal(t, "timeout while waiting for the lock", err2.Error())

	require.Contains(t, logger2buf.String(), "Waiting for a lock at "+file)
}

// Test that releasing a lock allows others to lock it again
func TestFileLockRelease(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "lock")
	logger1, logger1buf := ui.NewForTesting()
	logger2, logger2buf := ui.NewForTesting()

	lock1 := NewLock(file, 5*time.Millisecond)
	err1 := lock1.Acquire(context.Background(), logger1)
	require.NoError(t, err1)
	lock1.Release(logger1)

	lock2 := NewLock(file, 5*time.Millisecond)
	err2 := lock2.Acquire(context.Background(), logger2)
	require.NoError(t, err2)
	lock2.Release(logger2)

	require.Empty(t, logger1buf.String())
	require.Empty(t, logger2buf.String())
}

// Test that releasing a lock allows other waiting locks to proceed
func TestFileLockProceed(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
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
	require.NoError(t, err1)

	lock2 := NewLock(file, 5*time.Millisecond)
	err2 := lock2.Acquire(context.Background(), logger2)
	require.NoError(t, err2)
	lock2.Release(logger2)

	require.Empty(t, logger1buf.String())
	require.Contains(t, logger2buf.String(), "Waiting for a lock at "+file)
}
