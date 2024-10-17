package autoversion

import (
	"fmt"
	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"os"
	"os/exec"
	"path"
	"testing"
)

func Test_GitTagsAutoVersion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err, "could not create temp dir")

	err = runCommandInDir(tmpDir, "git", "init", ".")
	assert.NoError(t, err)

	err = os.WriteFile(path.Join(tmpDir, "README.md"), []byte("readme"), 0600)
	assert.NoError(t, err)

	err = runCommandInDir(tmpDir, "git", "config", "--local", "user.email", "test@example.com")
	assert.NoError(t, err)
	err = runCommandInDir(tmpDir, "git", "config", "--local", "user.name", "test")
	assert.NoError(t, err)
	err = runCommandInDir(tmpDir, "git", "add", ".")
	assert.NoError(t, err)

	err = runCommandInDir(tmpDir, "git", "commit", "-m", "initial commit")
	assert.NoError(t, err)

	err = runCommandInDir(tmpDir, "git", "tag", "v0.0.1")
	assert.NoError(t, err)

	err = runCommandInDir(tmpDir, "git", "tag", "v0.0.2")
	assert.NoError(t, err)

	latest, err := gitTagsAutoVersion(&manifest.AutoVersionBlock{
		GitTags:        tmpDir,
		VersionPattern: "v?(.*)",
	})

	assert.NoError(t, err)
	assert.Equal(t, latest, "0.0.2")
}

func runCommandInDir(dir string, cmd string, args ...string) error { //nolint: unparam
	command := exec.Command(cmd, args...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		return errors.WithStack(err)
	}
	return nil
}
