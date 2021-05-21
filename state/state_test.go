package state_test

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cashapp/hermit/manifest/manifesttest"
	"github.com/cashapp/hermit/ui"
	"github.com/stretchr/testify/require"
)

func TestCacheAndUnpackDownloadsOnlyWhenNeeded(t *testing.T) {
	calls := 0
	fixture := NewStateTestFixture(t).
		WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dat, err := ioutil.ReadFile("../archive/testdata/archive.tar.gz")
			require.NoError(t, err)
			_, err = w.Write(dat)
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
