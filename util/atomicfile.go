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
// magnitude slower than a plain write. That cost is judged not worth paying
// here for either of this helper's callers: internal/dao.UpdatePackage's
// writes (a cached etag and check timestamp) already sit behind a network
// round trip and aren't authoritative -- losing one to a crash just costs one
// extra upstream check next run -- and while Env.SetEnv/DelEnv's writes to
// the user's bin/hermit.hcl are more consequential (a crash could lose a
// just-persisted "hermit env" change), that's accepted as the cost of a
// single shared helper rather than special-casing fsync per caller.
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
