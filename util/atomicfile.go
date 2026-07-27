package util

import (
	"os"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
)

// AtomicWriteFile writes data to path atomically: it is written to a temp
// file in the same directory, then renamed into place. Unlike os.WriteFile,
// which truncates the existing file before writing, a concurrent reader can
// never observe an empty or partially-written file.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.WithStack(err)
	}
	tmpPath := tmp.Name()
	// Harmless once the rename below succeeds: nothing left to remove.
	defer os.Remove(tmpPath)

	_, writeErr := tmp.Write(data)
	var syncErr error
	if writeErr == nil {
		// Without this, a crash shortly after Rename can leave path pointing
		// at a temp file the filesystem never flushed, ie. a zero-length or
		// truncated file, despite the rename itself being durable.
		syncErr = tmp.Sync()
	}
	closeErr := tmp.Close()
	if writeErr != nil {
		return errors.WithStack(writeErr)
	}
	if syncErr != nil {
		return errors.WithStack(syncErr)
	}
	if closeErr != nil {
		return errors.WithStack(closeErr)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.Rename(tmpPath, path))
}
