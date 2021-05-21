package manifest

import (
	"os"
	"testing"

	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/stretchr/testify/require"
)

func TestLoader(t *testing.T) {
	stateDir := t.TempDir()
	srcs := sources.New(stateDir, []sources.Source{
		sources.NewLocalSource("test://", os.DirFS("./testdata")),
	})
	logger, _ := ui.NewForTesting()
	loader := NewLoader(logger, srcs)
	require.Len(t, srcs.Sources(), 1)
	manifest, err := loader.Get("protoc")
	require.NoError(t, err)
	require.Equal(t, "protoc is a compiler for protocol buffers definitions files.", manifest.Description)

	manifests, err := loader.All()
	require.NoError(t, err)
	require.Len(t, loader.Errors(), 1)
	require.Contains(t, loader.Errors(), "test:///corrupt.hcl")
	require.Len(t, manifests, 2)
}
