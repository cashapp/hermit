package manifest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.Len(t, srcs.Sources(), 1)
	manifest, err := loader.Load(l, "protoc")
	require.NoError(t, err)
	require.Equal(t, "protoc is a compiler for protocol buffers definitions files.", manifest.Description)

	manifests, err := loader.All()
	require.NoError(t, err)
	require.Len(t, loader.Errors(), 1)
	require.Contains(t, loader.Errors(), "test:///corrupt.hcl")
	require.Len(t, manifests, 2)
}
