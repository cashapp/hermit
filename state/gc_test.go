package state_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/manifest/manifesttest"
	"github.com/cashapp/hermit/ui"
)

const gcAge = 24 * time.Hour

func TestGCRemovesStateNotUsedWithinAge(t *testing.T) {
	fixture := NewStateTestFixture(t)
	defer fixture.Clean()
	state := fixture.State()
	root := state.Root()
	external := t.TempDir()

	mkdir(t, filepath.Join(state.PkgDir(), "old-1.0"))
	mkdir(t, filepath.Join(state.PkgDir(), "new-1.0"))
	mkdir(t, filepath.Join(state.PkgDir(), "hermit@stable"))
	mkdir(t, filepath.Join(state.PkgDir(), "new-1.0-123456")) // Interrupted unpack.
	symlink(t, filepath.Join(state.PkgDir(), "old-1.0"), filepath.Join(state.BinaryDir(), "old-1.0", "old"))
	symlink(t, filepath.Join(state.PkgDir(), "new-1.0"), filepath.Join(state.BinaryDir(), "new-1.0", "new"))
	symlink(t, external, filepath.Join(state.BinaryDir(), "external-1.0", "external"))
	for _, ref := range []string{"old-1.0", "new-1.0", "external-1.0", "gone-1.0", "hermit@stable"} {
		write(t, filepath.Join(root, "metadata", ref+".etag"))
	}
	mkdir(t, filepath.Join(state.EnvironmentsDir(), "active-abc", "oldfarm"))
	mkdir(t, filepath.Join(state.EnvironmentsDir(), "active-abc", "newfarm"))
	mkdir(t, filepath.Join(state.EnvironmentsDir(), "inactive-def", "oldfarm"))
	write(t, filepath.Join(root, "cache", "ab", "old.tar.gz"))
	write(t, filepath.Join(root, "cache", "ab", "new.tar.gz"))
	mkdir(t, filepath.Join(state.SourcesDir(), "oldsource"))
	mkdir(t, filepath.Join(state.SourcesDir(), "newsource"))

	age(t, root,
		"pkg/old-1.0", "pkg/hermit@stable", "pkg/new-1.0-123456",
		"metadata/old-1.0.etag", "metadata/new-1.0.etag", "metadata/external-1.0.etag",
		"metadata/gone-1.0.etag", "metadata/hermit@stable.etag",
		"environments/active-abc/oldfarm", "environments/inactive-def/oldfarm",
		"cache/ab/old.tar.gz",
		"sources/oldsource",
	)

	log, _ := ui.NewForTesting()
	assert.NoError(t, state.GC(log, gcAge))

	assert.Equal(t, []string{
		"binaries/external-1.0/external",
		"binaries/new-1.0/new",
		"cache/ab/new.tar.gz",
		"environments/active-abc/newfarm",
		"metadata/external-1.0.etag",
		"metadata/hermit@stable.etag",
		"metadata/new-1.0.etag",
		"pkg/hermit@stable",
		"pkg/new-1.0",
		"sources/newsource",
	}, leaves(t, root))
}

func TestCacheAndUnpackRecordsUsage(t *testing.T) {
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
	pkg := manifesttest.NewPkgBuilder(filepath.Join(state.PkgDir(), "test-1.0")).
		WithVersion("1.0").
		WithSource(fixture.Server.URL).
		Result()

	assert.NoError(t, state.CacheAndUnpack(log.Task("test"), pkg))
	age(t, state.PkgDir(), filepath.Base(pkg.Dest))
	assert.NoError(t, state.CacheAndUnpack(log.Task("test"), pkg))
	assert.NoError(t, state.GC(log, gcAge))
	_, err := os.Stat(pkg.Dest)
	assert.NoError(t, err, "a package used within the GC age must be kept")

	age(t, state.PkgDir(), filepath.Base(pkg.Dest))
	assert.NoError(t, state.GC(log, gcAge))
	_, err = os.Stat(pkg.Dest)
	assert.True(t, os.IsNotExist(err), "a package unused for the GC age must be removed")
	_, err = os.Stat(filepath.Join(state.BinaryDir(), pkg.Reference.String()))
	assert.True(t, os.IsNotExist(err), "binary links to a removed package must be removed")
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	assert.NoError(t, os.MkdirAll(dir, 0o700))
}

func write(t *testing.T, path string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	assert.NoError(t, os.WriteFile(path, nil, 0o600))
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	mkdir(t, filepath.Dir(link))
	assert.NoError(t, os.Symlink(target, link))
}

// age sets the modification time of each path under root to twice the GC age ago.
func age(t *testing.T, root string, paths ...string) {
	t.Helper()
	old := time.Now().Add(-2 * gcAge)
	for _, path := range paths {
		assert.NoError(t, os.Chtimes(filepath.Join(root, path), old, old))
	}
}

// leaves lists the empty directories, files and symlinks under root, excluding Hermit's own
// lock, relative to root.
func leaves(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if entries, err := os.ReadDir(path); err != nil || len(entries) > 0 || path == root {
				return err
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".lock" {
			return err
		}
		out = append(out, rel)
		return nil
	})
	assert.NoError(t, err)
	return out
}
