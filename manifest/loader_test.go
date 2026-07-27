package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
)

func TestLoader(t *testing.T) {
	l, _ := ui.NewForTesting()

	stateDir := t.TempDir()
	srcs := sources.New(stateDir, []sources.Source{
		sources.NewLocalSource("test://", os.DirFS("./testdata")),
	})
	loader := NewLoader(srcs)
	assert.Equal(t, len(srcs.Sources()), 1)
	manifest, err := loader.Load(l, "protoc")
	assert.NoError(t, err)
	assert.Equal(t, "protoc is a compiler for protocol buffers definitions files.", manifest.Description)

	manifests, err := loader.All()
	assert.NoError(t, err)
	assert.Equal(t, len(loader.Errors()), 1)
	assert.NotZero(t, loader.Errors()["test:///corrupt.hcl"])
	assert.Equal(t, len(manifests), 2)
}

// noopRunner is a util.CommandRunner that never actually runs anything. It's
// only used below to construct a GitSource whose backing directory is
// deliberately never populated, so Sync is never expected to be called.
type noopRunner struct{}

func (noopRunner) RunInDir(_ *ui.Task, _ string, _ ...string) error { return nil }

func TestLoaderMissingManifestIsUnknownPackage(t *testing.T) {
	l, _ := ui.NewForTesting()
	stateDir := t.TempDir()
	srcs := sources.New(stateDir, []sources.Source{
		sources.NewLocalSource("test://", os.DirFS("./testdata")),
	})
	loader := NewLoader(srcs)
	_, err := loader.Load(l, "does-not-exist")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnknownPackage))
	assert.False(t, errors.Is(err, sources.ErrSourceUnavailable))
}

// TestLoaderMissingSourceDirIsSourceUnavailable verifies that a GitSource
// whose backing directory has never been created (eg. because another
// Hermit process hasn't finished its initial sync yet) is reported as
// ErrSourceUnavailable, not folded into "unknown package" -- the whole point
// of the distinction is that Loader.Load treats the two differently.
func TestLoaderMissingSourceDirIsSourceUnavailable(t *testing.T) {
	stateDir := t.TempDir()
	git := sources.NewGitSource("git://missing", filepath.Join(stateDir, "src"), noopRunner{})
	srcs := sources.New(stateDir, []sources.Source{git})
	loader := NewLoader(srcs)

	_, err := loader.get("anything")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, sources.ErrSourceUnavailable))
}

// TestLoaderFallsBackToHealthySourceWhenAnotherIsUnavailable verifies that
// one unavailable source doesn't mask a package provided by another, healthy
// source: get() must keep searching remaining bundles rather than bailing
// out on the first unavailable one.
func TestLoaderFallsBackToHealthySourceWhenAnotherIsUnavailable(t *testing.T) {
	stateDir := t.TempDir()
	missing := sources.NewGitSource("git://missing", filepath.Join(stateDir, "missing-src"), noopRunner{})
	local := sources.NewLocalSource("test://", os.DirFS("./testdata"))
	srcs := sources.New(stateDir, []sources.Source{missing, local})
	loader := NewLoader(srcs)

	manifest, err := loader.get("protoc")
	assert.NoError(t, err)
	assert.Equal(t, "protoc is a compiler for protocol buffers definitions files.", manifest.Description)
}

// TestLoaderBuiltInSourceMissingManifestIsUnknownPackage guards against the
// pitfall where an in-memory source (vfs.InMemoryFS, used by BuiltInSource
// and MemSource) unconditionally returns fs.ErrNotExist with no backing
// directory to probe: it must never be misreported as ErrSourceUnavailable.
func TestLoaderBuiltInSourceMissingManifestIsUnknownPackage(t *testing.T) {
	builtin := sources.NewBuiltInSource(vfs.InMemoryFS(map[string]string{}))
	srcs := sources.New(t.TempDir(), []sources.Source{builtin})
	loader := NewLoader(srcs)

	_, err := loader.get("anything")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnknownPackage))
	assert.False(t, errors.Is(err, sources.ErrSourceUnavailable))
}
