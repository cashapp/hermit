package state_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/manifest/manifesttest"
	"github.com/cashapp/hermit/ui"
)

func TestCacheAndUnpackDownloadsOnlyWhenNeeded(t *testing.T) {
	calls := 0
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			assert.NoError(t, err)
			calls++
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).WithSource(fixture.Server.URL).Result()

	err := state.CacheAndUnpack(log.Task("test"), pkg)
	assert.NoError(t, err)
	err = state.CleanCache(log.Task("test"))
	assert.NoError(t, err)

	// Check that removing the cache does not re-download the package if it is extracted
	err = state.CacheAndUnpack(log.Task("test"), pkg)
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestCacheAndUnpackHooksRunOnMutablePackage(t *testing.T) {
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			assert.NoError(t, err)
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).
		WithTrigger(manifest.EventUnpack, &manifest.RenameAction{
			From: filepath.Join(state.PkgDir(), "file"),
			To:   filepath.Join(state.PkgDir(), "file_renamed"),
		}).
		WithSource(fixture.Server.URL).
		Result()

	err := state.CacheAndUnpack(log.Task("test"), pkg)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(state.PkgDir(), "file_renamed"))
	assert.NoError(t, err)

	info, err := os.Stat(state.PkgDir())
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0500), info.Mode()&0777, info.Mode().String())

	info, err = os.Stat(filepath.Join(state.PkgDir(), "file_renamed"))
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0500), info.Mode()&0777, info.Mode().String())
}

func TestLinksMissingBinaries(t *testing.T) {
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			assert.NoError(t, err)
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).
		WithBinaries("darwin_exe").
		WithSource(fixture.Server.URL).Result()

	err := state.CacheAndUnpack(log.Task("test"), pkg)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(state.BinaryDir(), pkg.Reference.String(), "darwin_exe"))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(state.BinaryDir(), pkg.Reference.String(), "linux_exe"))
	assert.Error(t, err, "linux_exe should not exist")

	pkg = manifesttest.NewPkgBuilder(state.PkgDir()).
		WithBinaries("darwin_exe", "linux_exe").
		WithSource(fixture.Server.URL).Result()

	err = state.CacheAndUnpack(log.Task("test"), pkg)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(state.BinaryDir(), pkg.Reference.String(), "darwin_exe"))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(state.BinaryDir(), pkg.Reference.String(), "linux_exe"))
	assert.NoError(t, err)
}

func TestCacheAndUnpackCreatesBinarySymlinks(t *testing.T) {
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			assert.NoError(t, err)
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).
		WithSource(fixture.Server.URL).
		Result()

	assert.NoError(t, state.CacheAndUnpack(log.Task("test"), pkg))
	_, err := os.Stat(filepath.Join(state.BinaryDir(), pkg.Reference.String(), "darwin_exe"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(state.BinaryDir(), pkg.Reference.String(), "linux_exe"))
	assert.NoError(t, err)
}

func TestUpdateSymlinks(t *testing.T) {
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			assert.NoError(t, err)
		}))
	defer fixture.Clean()
	state := fixture.State()
	log, _ := ui.NewForTesting()
	pkgDir := state.PkgDir()
	pkg := manifesttest.NewPkgBuilder(pkgDir).
		WithSource(fixture.Server.URL).
		WithName("test").
		Result()
	assert.NoError(t, state.CacheAndUnpack(log.Task("test"), pkg))
	newPkg := manifesttest.NewPkgBuilder(pkgDir + "-new").
		WithSource(fixture.Server.URL).
		WithName("test").
		Result()

	// unpack and make new links
	assert.NoError(t, state.CacheAndUnpack(log.Task("test"), newPkg))

	darwinExec := filepath.Join(state.BinaryDir(), newPkg.Reference.String(), "darwin_exe")
	linuxExec := filepath.Join(state.BinaryDir(), newPkg.Reference.String(), "linux_exe")
	darwinLink, err := os.Readlink(darwinExec)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(newPkg.Dest, "darwin_exe"), darwinLink)

	linuxLink, err := os.Readlink(linuxExec)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(newPkg.Dest, "linux_exe"), linuxLink)

}

func TestUpgrade(t *testing.T) {
	etagCounter := 0
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// always generate a new ETag to invalidate the cache
			w.Header().Set("ETag", fmt.Sprintf("etag-%d", etagCounter))
			etagCounter++
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			assert.NoError(t, err)

		}))
	defer fixture.Clean()
	state := fixture.State()
	log, _ := ui.NewForTesting()
	rootPkgDir := state.PkgDir()

	packageNames := []string{"hermit", "dune"}

	for _, name := range packageNames {
		t.Run(name, func(t *testing.T) {
			pkgDir := filepath.Join(rootPkgDir, name)
			renamedDir := filepath.Join(rootPkgDir, fmt.Sprintf(".%s.old", name))
			pkg := manifesttest.NewPkgBuilder(pkgDir).
				WithSource(fixture.Server.URL).
				WithName(name).
				WithChannel("stable").
				Result()
			assert.NoError(t, state.CacheAndUnpack(log.Task("test"), pkg))

			// Check package file exists
			_, err := os.Stat(pkgDir)
			assert.NoError(t, err)
			_, err = os.Stat(renamedDir)
			assert.Error(t, err)

			// Update cache with new ETag
			_, err = state.CacheAndDigest(log.Task("test"), pkg)
			assert.NoError(t, err)

			// Run upgrade
			err = state.UpgradeChannel(log.Task("test"), pkg)
			assert.NoError(t, err)

			_, err = os.Stat(pkgDir)
			assert.NoError(t, err)
			// For hermit, we should have renamed the old directory
			if name == "hermit" {
				_, err = os.Stat(renamedDir)
				assert.NoError(t, err)
			} else {
				// For dune, we should not have renamed the old directory
				_, err = os.Stat(renamedDir)
				assert.Error(t, err)
			}

			// Another upgrade to make sure we delete the old hermit directory
			_, err = state.CacheAndDigest(log.Task("test"), pkg)
			assert.NoError(t, err)
		})
	}

}
