package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestSwapDir(t *testing.T) {
	dir := t.TempDir()
	finalDest := filepath.Join(dir, "final")
	assert.NoError(t, os.MkdirAll(finalDest, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(finalDest, "old.txt"), []byte("old"), 0600))

	src := filepath.Join(dir, "new")
	assert.NoError(t, os.MkdirAll(src, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("new"), 0600))

	assert.NoError(t, SwapDir(src, finalDest))

	_, err := os.Stat(filepath.Join(finalDest, "new.txt"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(finalDest, "old.txt"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(finalDest + DirSwapAsideSuffix)
	assert.True(t, os.IsNotExist(err))
}

// TestSwapDirRecoversFromMissingSource verifies SwapDir cleans up a stale
// ".old" left behind by a process that crashed mid-swap, and that finalDest
// never has an intervening moment where it doesn't exist for a caller that
// only checks before and after (a genuine no-gap guarantee needs a
// concurrent reader, which is exercised at the sources.GitSource level).
func TestSwapDirRecoversFromMissingSource(t *testing.T) {
	dir := t.TempDir()
	finalDest := filepath.Join(dir, "final")
	assert.NoError(t, os.MkdirAll(finalDest, 0700))

	// Simulate a previous crash mid-swap: a stale ".old" already exists.
	aside := finalDest + DirSwapAsideSuffix
	assert.NoError(t, os.MkdirAll(aside, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(aside, "junk"), []byte("junk"), 0600))

	src := filepath.Join(dir, "new")
	assert.NoError(t, os.MkdirAll(src, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("new"), 0600))

	assert.NoError(t, SwapDir(src, finalDest))

	_, err := os.Stat(filepath.Join(finalDest, "new.txt"))
	assert.NoError(t, err)
	_, err = os.Stat(aside)
	assert.True(t, os.IsNotExist(err))
}

// TestSwapDirNoPreviousDest verifies SwapDir works when finalDest doesn't
// exist yet at all (the common case: first-ever creation of a directory).
func TestSwapDirNoPreviousDest(t *testing.T) {
	dir := t.TempDir()
	finalDest := filepath.Join(dir, "final")

	src := filepath.Join(dir, "new")
	assert.NoError(t, os.MkdirAll(src, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("new"), 0600))

	assert.NoError(t, SwapDir(src, finalDest))

	_, err := os.Stat(filepath.Join(finalDest, "new.txt"))
	assert.NoError(t, err)
}

func TestRemoveAllAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "victim")
	assert.NoError(t, os.MkdirAll(target, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(target, "f"), []byte("data"), 0600))

	assert.NoError(t, RemoveAllAtomic(target))

	_, err := os.Stat(target)
	assert.True(t, os.IsNotExist(err))
	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(entries), "no scratch entries should be left behind")
}

// TestRemoveAllAtomicMissingTarget verifies RemoveAllAtomic is a no-op (not
// an error) when the target doesn't exist, matching os.RemoveAll's
// semantics.
func TestRemoveAllAtomicMissingTarget(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, RemoveAllAtomic(filepath.Join(dir, "does-not-exist")))
}
