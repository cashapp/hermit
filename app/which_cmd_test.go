package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/kong"

	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/ui"
)

func TestWhichResolvesLinkedBinaryGlobs(t *testing.T) {
	f := hermittest.NewEnvTestFixture(t, nil).WithManifests(map[string]string{
		"tool.hcl": `
			description = ""
			binaries = ["*_exe"]
			source = "../archive/testdata/archive.tar.gz"
			on exec {
				run { cmd = "/bin/sh" args = ["-c", "exit 1"] }
			}
			version "1.0.0" "2.0.0" {}
		`,
	})
	binDir := f.Env.BinDir()
	assert.NoError(t, os.Symlink("hermit", filepath.Join(binDir, ".tool-1.0.0.pkg")))
	assert.NoError(t, os.Symlink(".tool-1.0.0.pkg", filepath.Join(binDir, "darwin_exe")))
	assert.NoError(t, os.Symlink("darwin_exe", filepath.Join(binDir, "alias")))

	for _, binary := range []string{"darwin_exe", "alias", filepath.Join(binDir, "darwin_exe")} {
		t.Run(binary, func(t *testing.T) {
			l, output := ui.NewForTesting()
			l.SetLevel(ui.LevelInfo)
			l.SetProgressBarEnabled(false)
			cmd := whichCmd{Binary: binary}
			assert.NoError(t, cmd.Run(nil, l, f.Env, f.State))
			expected := filepath.Join(f.RootDir(), "tool-1.0.0", "darwin_exe")
			assert.Equal(t, expected+"\n", output.String())
			_, err := os.Stat(expected)
			assert.NoError(t, err)
		})
	}
}

func TestWhichDoesNotResolveUnlinkedBinaries(t *testing.T) {
	f := hermittest.NewEnvTestFixture(t, nil).WithManifests(map[string]string{
		"darwin_exe.hcl": `
			description = ""
			binaries = ["darwin_exe"]
			source = "../archive/testdata/archive.tar.gz"
			version "1.0.0" {}
		`,
	})
	exitCode := 0
	ctx := &kong.Context{Kong: &kong.Kong{Exit: func(code int) { exitCode = code }}}
	l, output := ui.NewForTesting()
	cmd := whichCmd{Binary: "darwin_exe"}
	assert.NoError(t, cmd.Run(ctx, l, f.Env, f.State))
	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "", output.String())
	_, err := os.Stat(filepath.Join(f.RootDir(), "darwin_exe-1.0.0"))
	assert.True(t, os.IsNotExist(err))
}

func TestWhichReportsInvalidBinaryLinks(t *testing.T) {
	f := hermittest.NewEnvTestFixture(t, nil)
	assert.NoError(t, os.Symlink("hermit", filepath.Join(f.Env.BinDir(), "invalid")))
	l, output := ui.NewForTesting()
	cmd := whichCmd{Binary: "invalid"}
	assert.Error(t, cmd.Run(nil, l, f.Env, f.State))
	assert.Equal(t, "", output.String())
}
