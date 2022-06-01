package sources_test

import (
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"testing"
)

type FailingGit struct {
	err error
}

func (f *FailingGit) RunInDir(_ *ui.Task, _ string, _ ...string) error {
	return f.err
}

func TestGitDoesNotRemoveSourceAfterSyncFailure(t *testing.T) {
	git := &FailingGit{}
	sourceDir := t.TempDir()
	source := sources.NewGitSource("git://test", sourceDir, git)

	// Create the initial directory for sources by successfully syncing
	u, _ := ui.NewForTesting()
	err := source.Sync(u, true)
	require.NoError(t, err)
	files, err := ioutil.ReadDir(sourceDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	gitDir := files[0].Name()

	// Fail the sync
	git.err = errors.New("failing git fails")
	err = source.Sync(u, true)

	// no error as it was not an initial clone
	require.NoError(t, err)

	// the directory should still be in place after git failed to update
	files, err = ioutil.ReadDir(sourceDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, gitDir, files[0].Name())

}
