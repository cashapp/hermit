//go:build !windows

package util_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/util"
)

func TestSystemCommandRestoresPathBeforeHermitActivation(t *testing.T) {
	hostDir := t.TempDir()
	hostGit := filepath.Join(hostDir, "git")
	assert.NoError(t, os.WriteFile(hostGit, []byte("#!/bin/sh\nprintf 'host:%s' \"$PATH\"\n"), 0700))

	envRoot := t.TempDir()
	envBin := filepath.Join(envRoot, "bin")
	assert.NoError(t, os.Mkdir(envBin, 0700))
	attackerGit := filepath.Join(envBin, "git")
	assert.NoError(t, os.WriteFile(attackerGit, []byte("#!/bin/sh\nprintf attacker-controlled\n"), 0700))

	ops, err := envars.MarshalOps(envars.Ops{&envars.Prepend{Name: "PATH", Value: envBin}})
	assert.NoError(t, err)
	t.Setenv("HERMIT_ENV", envRoot)
	t.Setenv("HERMIT_ENV_OPS", string(ops))
	t.Setenv("PATH", envBin+string(os.PathListSeparator)+hostDir)

	cmd, err := util.SystemCommand("git")
	assert.NoError(t, err)
	assert.Equal(t, hostGit, cmd.Path)

	out, err := cmd.Output()
	assert.NoError(t, err)
	assert.Equal(t, "host:"+hostDir, string(out))
}

func TestSystemCommandFailsClosedForInvalidHermitEnvOps(t *testing.T) {
	t.Setenv("HERMIT_ENV_OPS", "not JSON")

	_, err := util.SystemCommand("git")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "failed to restore PATH before Hermit activation"))
}

func TestSystemCommandRejectsPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "git")
	_, err := util.SystemCommand(path)
	assert.EqualError(t, err, `system command must be a bare name: "`+path+`"`)
}
