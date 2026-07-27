package dao

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func TestGetPackageMissing(t *testing.T) {
	d, err := Open(t.TempDir())
	assert.NoError(t, err)

	pkg, err := d.GetPackage("does-not-exist")
	assert.NoError(t, err)
	assert.Zero(t, pkg)
}

func TestUpdateAndGetPackageRoundTrip(t *testing.T) {
	d, err := Open(t.TempDir())
	assert.NoError(t, err)

	checkedAt := time.Date(2024, 3, 14, 15, 9, 26, 0, time.UTC)
	assert.NoError(t, d.UpdatePackage("pkg@1.0.0", &Package{
		Etag:            "some-etag",
		UpdateCheckedAt: checkedAt,
	}))

	got, err := d.GetPackage("pkg@1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "some-etag", got.Etag)
	assert.True(t, checkedAt.Equal(got.UpdateCheckedAt), "expected %s, got %s", checkedAt, got.UpdateCheckedAt)
}

// TestGetPackageLegacyFormat verifies that a metadata file written by a
// Hermit version prior to the introduction of the JSON envelope (ie. one
// containing only the raw etag, with no recorded check time) is still read
// correctly, falling back to the file's mtime for UpdateCheckedAt exactly as
// GetPackage always did previously.
func TestGetPackageLegacyFormat(t *testing.T) {
	d, err := Open(t.TempDir())
	assert.NoError(t, err)

	path := d.metadataPath("legacy@1.0.0")
	assert.NoError(t, os.WriteFile(path, []byte("legacy-raw-etag"), 0600))
	mtime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	assert.NoError(t, os.Chtimes(path, mtime, mtime))

	got, err := d.GetPackage("legacy@1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "legacy-raw-etag", got.Etag)
	assert.True(t, mtime.Equal(got.UpdateCheckedAt), "expected %s, got %s", mtime, got.UpdateCheckedAt)
}

// TestUpdatePackageAtomicNoTornRead is the reproducer/regression test for the
// torn-read bug: os.WriteFile truncates the existing file before writing the
// new content, so a GetPackage racing an UpdatePackage could previously
// observe an empty or partial etag. UpdatePackage now writes to a temp file
// and renames it into place, so every concurrent read must see either the
// old, complete value or a new, complete value -- never anything in between.
func TestUpdatePackageAtomicNoTornRead(t *testing.T) {
	d, err := Open(t.TempDir())
	assert.NoError(t, err)
	const pkgRef = "pkg@1.0.0"

	// Give readers something to see from the very first iteration.
	assert.NoError(t, d.UpdatePackage(pkgRef, &Package{Etag: "etag-0", UpdateCheckedAt: time.Now()}))

	valid := map[string]bool{"etag-0": true}
	for i := range 50 {
		valid[fmt.Sprintf("etag-%d", i+1)] = true
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	readerErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				readerErr <- nil
				return
			default:
			}
			pkg, err := d.GetPackage(pkgRef)
			if err != nil {
				readerErr <- err
				return
			}
			if pkg != nil && !valid[pkg.Etag] {
				readerErr <- fmt.Errorf("observed torn/unexpected etag: %q", pkg.Etag)
				return
			}
		}
	}()

	for i := range 50 {
		assert.NoError(t, d.UpdatePackage(pkgRef, &Package{
			Etag:            fmt.Sprintf("etag-%d", i+1),
			UpdateCheckedAt: time.Now(),
		}))
	}
	close(stop)
	wg.Wait()
	assert.NoError(t, <-readerErr)
}

// TestUpdatePackageLeavesNoTempFiles guards against leaking the scratch temp
// file UpdatePackage writes before renaming into place.
func TestUpdatePackageLeavesNoTempFiles(t *testing.T) {
	stateDir := t.TempDir()
	d, err := Open(stateDir)
	assert.NoError(t, err)

	assert.NoError(t, d.UpdatePackage("pkg@1.0.0", &Package{Etag: "some-etag", UpdateCheckedAt: time.Now()}))

	entries, err := os.ReadDir(filepath.Join(stateDir, "metadata"))
	assert.NoError(t, err)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("leaked temp file: %s", entry.Name())
		}
	}
}
