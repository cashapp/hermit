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
//
// This deliberately does not fsync the temp file before renaming it: doing so
// closes a narrow crash-durability gap (a crash between a successful-looking
// write and the underlying data actually reaching disk could otherwise leave
// path pointing at a zero-length or truncated file), but on macOS, Go's
// File.Sync issues fcntl(F_FULLFSYNC), which is roughly two orders of
// magnitude slower than a plain write -- and this is called from
// internal/dao.UpdatePackage on Hermit's "exec" hot path. The data this
// protects (a cached etag and check timestamp) is not authoritative state:
// losing it to a crash just costs one extra upstream check on the next run,
// which doesn't justify that cost on every invocation.
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
