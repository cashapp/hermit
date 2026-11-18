package state_test

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/manifest/manifesttest"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

func TestCleanPackagesDoesNotChmodSymlinkTargets(t *testing.T) {
	fixture := NewStateTestFixture(t)
	defer fixture.Clean()
	state := fixture.State()

	target := filepath.Join(t.TempDir(), "target")
	assert.NoError(t, os.WriteFile(target, nil, 0600))

	packageDir := filepath.Join(state.PkgDir(), "malicious-package")
	assert.NoError(t, os.MkdirAll(packageDir, 0700))
	assert.NoError(t, os.Symlink(target, filepath.Join(packageDir, "link")))

	log, _ := ui.NewForTesting()
	assert.NoError(t, state.CleanPackages(log))

	_, err := os.Lstat(packageDir)
	assert.True(t, os.IsNotExist(err), "package directory should be removed")
	info, err := os.Stat(target)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "symlink target permissions must not change")
}

func TestCacheAndUnpackDownloadsOnlyWhenNeeded(t *testing.T) {
	calls := 0
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close()
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

// channelServer serves the test archive with the given ETag and counts HEAD and GET requests.
func channelServer(t *testing.T, etag *string, heads, gets *int) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", *etag)
		if r.Method == http.MethodHead {
			*heads++
			return
		}
		*gets++
		fr, err := os.Open("../archive/testdata/archive.tar.gz")
		assert.NoError(t, err)
		defer fr.Close()
		_, err = io.Copy(w, fr)
		assert.NoError(t, err)
	})
}

// newChannelPackage returns test@stable as a freshly resolved package, as each Hermit process sees it.
func newChannelPackage(sta *state.State, source string) *manifest.Package {
	pkg := manifesttest.NewPkgBuilder(filepath.Join(sta.PkgDir(), "test@stable")).
		WithChannel("stable").
		WithUpdateInterval(time.Hour).
		WithSource(source).
		Result()
	sta.ReadPackageState(pkg)
	return pkg
}

// The first use of a channel package used to count as an overdue update
// check. It then evicted the archive and package directory that concurrent
// first uses were extracting or running, and reinstalled them.
func TestFirstChannelUseDoesNotReinstall(t *testing.T) {
	tests := []struct {
		name string
		// The package directory was restored from a CI cache without the download cache or metadata.
		restored bool
		wantGets int
	}{
		{name: "Downloaded", wantGets: 1},
		{name: "RestoredWithoutMetadata", restored: true, wantGets: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etag := "current"
			heads, gets := 0, 0
			fixture := NewStateTestFixture(t).WithHTTPHandler(channelServer(t, &etag, &heads, &gets))
			defer fixture.Clean()
			sta := fixture.State()
			log, _ := ui.NewForTesting()

			if test.restored {
				assert.NoError(t, sta.CacheAndUnpack(log.Task("test"), newChannelPackage(sta, fixture.Server.URL)))
				assert.NoError(t, sta.CleanCache(log))
				assert.NoError(t, os.RemoveAll(filepath.Join(sta.Root(), "metadata", "test@stable.etag")))
				assert.NoError(t, os.RemoveAll(sta.BinaryDir()))
				gets = 0
			}

			pkg := newChannelPackage(sta, fixture.Server.URL)
			assert.NoError(t, sta.CacheAndUnpack(log.Task("test"), pkg))
			installed, err := os.Stat(pkg.Dest)
			assert.NoError(t, err)

			// Exec resolves the package again before checking the channel.
			assert.NoError(t, sta.EnsureChannelIsUpToDate(log, newChannelPackage(sta, fixture.Server.URL)))

			assert.Equal(t, [2]int{0, test.wantGets}, [2]int{heads, gets}, "[HEAD, GET] requests")
			after, err := os.Stat(pkg.Dest)
			assert.NoError(t, err)
			assert.True(t, os.SameFile(installed, after), "package was reinstalled")
		})
	}
}

// Processes that find the same channel update must not each reinstall the
// package, and must never remove the cached archive another process may be
// about to extract.
func TestUpgradeChannelSkipsVersionInstalledByAnotherProcess(t *testing.T) {
	etag := "v1"
	heads, gets := 0, 0
	fixture := NewStateTestFixture(t).WithHTTPHandler(channelServer(t, &etag, &heads, &gets))
	defer fixture.Clean()
	sta := fixture.State()
	log, _ := ui.NewForTesting()
	assert.NoError(t, sta.CacheAndUnpack(log.Task("test"), newChannelPackage(sta, fixture.Server.URL)))

	etag = "v2"
	first := newChannelPackage(sta, fixture.Server.URL)
	second := newChannelPackage(sta, fixture.Server.URL)
	assert.NoError(t, sta.UpgradeChannel(log.Task("test"), first))
	upgraded, err := os.Stat(first.Dest)
	assert.NoError(t, err)

	assert.NoError(t, sta.UpgradeChannel(log.Task("test"), second))
	after, err := os.Stat(second.Dest)
	assert.NoError(t, err)
	assert.True(t, os.SameFile(upgraded, after), "package was reinstalled")
	assert.Equal(t, "v2", second.ETag)
	_, err = os.Stat(filepath.Join(sta.Root(), "cache", cache.BasePath("", redact.URL(fixture.Server.URL))))
	assert.NoError(t, err, "cached archive was removed")
}

func TestCacheAndUnpackFromHTTPMirrorDownloadsOnce(t *testing.T) {
	calls := 0
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/mirror.tar.gz" {
				http.NotFound(w, r)
				return
			}
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close()
			_, err = io.Copy(w, fr)
			assert.NoError(t, err)
			calls++
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).WithSource(fixture.Server.URL + "/source.tar.gz").Result()
	pkg.Mirrors = []redact.URL{redact.URL(fixture.Server.URL + "/mirror.tar.gz")}

	err := state.CacheAndUnpack(log.Task("test"), pkg)
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestCacheAndUnpackFromGitMirror(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo.git")
	assert.NoError(t, os.MkdirAll(repo, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(repo, "hello"), []byte("#!/bin/sh\n"), 0700))
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "add", "hello"},
		{"-C", repo, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "init"},
		{"-C", repo, "tag", "v1.0.0"},
	} {
		out, err := exec.Command("git", args...).CombinedOutput()
		assert.NoError(t, err, string(out))
	}
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
	defer fixture.Clean()
	state := fixture.State()

	log, _ := ui.NewForTesting()
	pkg := manifesttest.NewPkgBuilder(state.PkgDir()).
		WithSource(fixture.Server.URL + "/source.tar.gz").
		WithBinaries("hello").
		Result()
	pkg.Mirrors = []redact.URL{redact.URL("file://" + repo + "#v1.0.0")}

	err := state.CacheAndUnpack(log.Task("test"), pkg)
	assert.NoError(t, err)
}

func TestCacheAndUnpackHooksRunOnMutablePackage(t *testing.T) {
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fr, err := os.Open("../archive/testdata/archive.tar.gz")
			assert.NoError(t, err)
			defer fr.Close()
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
			defer fr.Close()
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
			defer fr.Close()
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
			defer fr.Close()
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
