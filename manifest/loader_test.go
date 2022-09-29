package manifest

import (
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
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
