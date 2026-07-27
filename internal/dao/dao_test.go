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

// TestGetPackageMissingCheckedAtSidecar verifies that a metadata directory
// containing only the raw etag file, with no ".checked" sidecar, is still
// read correctly, falling back to the etag file's mtime for
// UpdateCheckedAt. This is the on-disk state left by a Hermit version prior
// to the introduction of the sidecar (which wrote only the raw etag, in
// exactly this format) -- the two are indistinguishable, which is the point:
// an older Hermit binary sharing this state directory can still read and
// write the etag file unmodified.
func TestGetPackageMissingCheckedAtSidecar(t *testing.T) {
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

// TestOpenSweepsStaleScratchFiles verifies that Open cleans up an old,
// abandoned ".tmp-*" file left behind by a process killed mid-write, but
// leaves a recent one alone (it may belong to a write still in flight in
// another process).
func TestOpenSweepsStaleScratchFiles(t *testing.T) {
	stateDir := t.TempDir()
	metadataDir := filepath.Join(stateDir, "metadata")
	assert.NoError(t, os.MkdirAll(metadataDir, 0700))

	stale := filepath.Join(metadataDir, "pkg@1.0.0.etag.tmp-stale")
	assert.NoError(t, os.WriteFile(stale, []byte("abandoned"), 0600))
	old := time.Now().Add(-48 * time.Hour)
	assert.NoError(t, os.Chtimes(stale, old, old))

	fresh := filepath.Join(metadataDir, "pkg@2.0.0.etag.tmp-fresh")
	assert.NoError(t, os.WriteFile(fresh, []byte("in-flight"), 0600))

	_, err := Open(stateDir)
	assert.NoError(t, err)

	_, err = os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "stale scratch file should have been swept")
	_, err = os.Stat(fresh)
	assert.NoError(t, err, "recent scratch file should not have been swept")
}

// TestUpdatePackageLeavesNoTempFiles guards against leaking the scratch temp
// files UpdatePackage writes before renaming into place -- there are two
// atomic writes per call (the ".etag" file and the ".checked" sidecar), each
// with its own temp file.
func TestUpdatePackageLeavesNoTempFiles(t *testing.T) {
	stateDir := t.TempDir()
	d, err := Open(stateDir)
	assert.NoError(t, err)

	assert.NoError(t, d.UpdatePackage("pkg@1.0.0", &Package{Etag: "some-etag", UpdateCheckedAt: time.Now()}))

	entries, err := os.ReadDir(filepath.Join(stateDir, "metadata"))
	assert.NoError(t, err)
	var names []string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("leaked temp file: %s", entry.Name())
		}
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"pkg@1.0.0.checked", "pkg@1.0.0.etag"}, names)
}
