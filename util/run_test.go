//go:build !windows

package util_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/util"
)

func TestSystemCommandIgnoresProcessPath(t *testing.T) {
	attackerDir := t.TempDir()
	attackerShell := filepath.Join(attackerDir, "sh")
	assert.NoError(t, os.WriteFile(attackerShell, []byte("#!/bin/sh\necho attacker-controlled\n"), 0700))
	t.Setenv("PATH", attackerDir)

	cmd, err := util.SystemCommand("sh", "-c", `printf '%s' "$PATH"`)
	assert.NoError(t, err)
	assert.NotEqual(t, attackerShell, cmd.Path)

	out, err := cmd.Output()
	assert.NoError(t, err)
	assert.False(t, strings.Contains(string(out), attackerDir), "system helper inherited attacker-controlled PATH")
}

func TestSystemCommandRejectsPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "git")
	_, err := util.SystemCommand(path)
	assert.EqualError(t, err, `system command must be a bare name: "`+path+`"`)
}
