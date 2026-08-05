package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestSymlinkActionApply(t *testing.T) {
	t.Run("CreatesSymlink", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "from")
		to := filepath.Join(dir, "to")
		assert.NoError(t, os.WriteFile(from, []byte("content"), 0600))

		action := &SymlinkAction{From: from, To: to}
		assert.NoError(t, action.Apply(nil))

		target, err := os.Readlink(to)
		assert.NoError(t, err)
		assert.Equal(t, from, target)
	})

	t.Run("IsIdempotent", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "from")
		to := filepath.Join(dir, "to")
		assert.NoError(t, os.WriteFile(from, []byte("content"), 0600))

		action := &SymlinkAction{From: from, To: to}
		assert.NoError(t, action.Apply(nil))
		assert.NoError(t, action.Apply(nil))

		target, err := os.Readlink(to)
		assert.NoError(t, err)
		assert.Equal(t, from, target)
	})

	t.Run("ReplacesSymlinkToDifferentTarget", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "from")
		other := filepath.Join(dir, "other")
		to := filepath.Join(dir, "to")
		assert.NoError(t, os.WriteFile(from, []byte("content"), 0600))
		assert.NoError(t, os.WriteFile(other, []byte("other"), 0600))
		assert.NoError(t, os.Symlink(other, to))

		action := &SymlinkAction{From: from, To: to}
		assert.NoError(t, action.Apply(nil))

		target, err := os.Readlink(to)
		assert.NoError(t, err)
		assert.Equal(t, from, target)
	})

	t.Run("ReplacesExistingFile", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "from")
		to := filepath.Join(dir, "to")
		assert.NoError(t, os.WriteFile(from, []byte("content"), 0600))
		assert.NoError(t, os.WriteFile(to, []byte("existing"), 0600))

		action := &SymlinkAction{From: from, To: to}
		assert.NoError(t, action.Apply(nil))

		target, err := os.Readlink(to)
		assert.NoError(t, err)
		assert.Equal(t, from, target)
	})

	t.Run("RefusesToReplaceDirectory", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "from")
		to := filepath.Join(dir, "to")
		assert.NoError(t, os.WriteFile(from, []byte("content"), 0600))
		assert.NoError(t, os.Mkdir(to, 0700))

		action := &SymlinkAction{From: from, To: to}
		err := action.Apply(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "destination exists and is a directory")

		info, err := os.Stat(to)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}
