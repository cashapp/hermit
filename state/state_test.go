package state_test

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/manifest/manifesttest"
	"github.com/cashapp/hermit/redact"
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
