package state_test

import (
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
