package state_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/manifest/manifesttest"
	"github.com/cashapp/hermit/ui"
)

func TestCacheAndUnpackDownloadsOnlyWhenNeeded(t *testing.T) {
	calls := 0
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			require.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			require.NoError(t, err)
			calls++
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).WithSource(fixture.Server.URL).Result()

	err := state.CacheAndUnpack(log.Task("test"), pkg)
	require.NoError(t, err)
	err = state.CleanCache(log.Task("test"))
	require.NoError(t, err)

	// Check that removing the cache does not re-download the package if it is extracted
	err = state.CacheAndUnpack(log.Task("test"), pkg)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestCacheAndUnpackHooksRunOnMutablePackage(t *testing.T) {
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			require.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			require.NoError(t, err)
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
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(state.PkgDir(), "file_renamed"))

	info, err := os.Stat(state.PkgDir())
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0500), info.Mode()&0777, info.Mode().String())

	info, err = os.Stat(filepath.Join(state.PkgDir(), "file_renamed"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0500), info.Mode()&0777, info.Mode().String())
}

func TestCacheAndUnpackCreatesBinarySymlinks(t *testing.T) {
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			require.NoError(t, err)
			defer fr.Close() // nolint
			_, err = io.Copy(w, fr)
			require.NoError(t, err)
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).
		WithSource(fixture.Server.URL).
		Result()

	require.NoError(t, state.CacheAndUnpack(log.Task("test"), pkg))
	require.FileExists(t, filepath.Join(state.BinaryDir(), pkg.Reference.String(), "darwin_exe"))
	require.FileExists(t, filepath.Join(state.BinaryDir(), pkg.Reference.String(), "linux_exe"))
}
