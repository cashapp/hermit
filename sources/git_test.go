package sources_test

import (
	"os"
	"strings"
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

// sourceDirs returns the names of real source directories in dir, ignoring
// Hermit's lock files and sync scratch directories. Source directory names
// are bare hex SHA256 hashes (see util.Hash); every scratch/lock entry
// contains a ".", so this distinction is unambiguous.
func sourceDirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	var dirs []string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".") {
			continue
		}
		dirs = append(dirs, entry.Name())
	}
	return dirs
}

func TestGitDoesNotRemoveSourceAfterSyncFailure(t *testing.T) {
	git := &FailingGit{}
	sourceDir := t.TempDir()
	source := sources.NewGitSource("git://test", sourceDir, git)

	// Create the initial directory for sources by successfully syncing
	u, _ := ui.NewForTesting()
	_, err := source.Sync(u, true)
	assert.NoError(t, err)
	dirs := sourceDirs(t, sourceDir)
	assert.Equal(t, len(dirs), 1)
	gitDir := dirs[0]

	// Fail the sync
	git.err = errors.New("failing git fails")
	_, err = source.Sync(u, true)

	// no error as it was not an initial clone
	assert.NoError(t, err)

	// the directory should still be in place after git failed to update
	dirs = sourceDirs(t, sourceDir)
	assert.Equal(t, len(dirs), 1)
	assert.Equal(t, gitDir, dirs[0])
}
