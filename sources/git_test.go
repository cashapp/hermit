package sources_test

import (
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
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
	assert.NoError(t, err)
	files, err := os.ReadDir(sourceDir)
	assert.NoError(t, err)
	assert.Equal(t, len(files), 1)
	gitDir := files[0].Name()

	// Fail the sync
	git.err = errors.New("failing git fails")
	err = source.Sync(u, true)

	// no error as it was not an initial clone
	assert.NoError(t, err)

	// the directory should still be in place after git failed to update
	files, err = os.ReadDir(sourceDir)
	assert.NoError(t, err)
	assert.Equal(t, len(files), 1)
	assert.Equal(t, gitDir, files[0].Name())

}
