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
	closeErr := tmp.Close()
	if writeErr != nil {
		return errors.WithStack(writeErr)
	}
	if closeErr != nil {
		return errors.WithStack(closeErr)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.Rename(tmpPath, path))
}
