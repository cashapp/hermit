package flock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"golang.org/x/sync/errgroup"
)

func TestFlock(t *testing.T) {
	t.Cleanup(func() { getPID = os.Getpid })

	dir := t.TempDir()
	lockfile := filepath.Join(dir, "lock")

	ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*200)
	defer cancel()

	currentPID := 123
	getPID = func() int { currentPID++; return currentPID }

	release, err := Acquire(ctx, lockfile, "test")
	assert.NoError(t, err)

	// Second lock will fail because it's from a different PID.
	_, err = Acquire(ctx, lockfile, "test")
	assert.Error(t, err)

	err = release()
	assert.NoError(t, err)
}

func TestRecursiveFlock(t *testing.T) {
	getPID = func() int { return 123 }

	dir := t.TempDir()
	lockfile := filepath.Join(dir, "lock")

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second)
	defer cancel()

	releaseb, err := Acquire(ctx, lockfile, "test2")
	assert.NoError(t, err)

	releasec, err := Acquire(ctx, lockfile, "test2")
	assert.NoError(t, err)
	err = releasec()
	assert.NoError(t, err)

	err = releaseb()
	assert.NoError(t, err)
}

// Test that a second flock will acquire after the first is released.
func TestContinueFlock(t *testing.T) {
	currentPID := 123
	getPID = func() int { currentPID++; return currentPID }

	dir := t.TempDir()
	lockfile := filepath.Join(dir, "lock")

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second)
	defer cancel()

	release, err := Acquire(ctx, lockfile, "test")
	assert.NoError(t, err)

	wg, ctx := errgroup.WithContext(ctx)
	wg.Go(func() error {
		release, err := Acquire(ctx, lockfile, "test")
		if err != nil {
			return err
		}
		return release()
	})
	time.Sleep(time.Millisecond * 100)
	err = release()
	assert.NoError(t, err)

	err = wg.Wait()
	assert.NoError(t, err)
}
