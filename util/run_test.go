//go:build !windows

package util_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
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

func TestCommandRunnerRedactsSensitiveArgsInErrors(t *testing.T) {
	p, _ := ui.NewForTesting()
	runner := &util.RealCommandRunner{}
	err := runner.RunInDir(p.Task("test"), t.TempDir(),
		redact.Plain("ls"), redact.URL("https://x-access-token:sekret@example.com/repo.git"))
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")
	assert.Contains(t, err.Error(), "https://example.com/repo.git")
}

func TestCommandRunnerScrubsSensitiveArgsFromRelayedOutput(t *testing.T) {
	p, buf := ui.NewForTesting()
	runner := &util.RealCommandRunner{}
	err := runner.RunInDir(p.Task("test"), t.TempDir(),
		redact.Plain("echo"), redact.URL("https://x-access-token:sekret@example.com/repo.git"))
	assert.NoError(t, err)
	assert.NotContains(t, buf.String(), "sekret")
	assert.Contains(t, buf.String(), "https://example.com/repo.git")
}

func TestCommandRunnerCaptureScrubsSensitiveArgs(t *testing.T) {
	p, buf := ui.NewForTesting()
	runner := &util.RealCommandRunner{}
	url := redact.URL("https://x-access-token:sekret@example.com/repo.git")

	out, err := runner.CaptureInDir(p, t.TempDir(),
		redact.Plain("sh"), redact.Plain("-c"), redact.Plain(`echo "$0"`), url)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/repo.git\n", string(out))
	assert.NotContains(t, buf.String(), "sekret")

	_, err = runner.CaptureInDir(nil, t.TempDir(),
		redact.Plain("sh"), redact.Plain("-c"), redact.Plain(`echo "$0"; exit 1`), url)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret")
	assert.Contains(t, err.Error(), "https://example.com/repo.git")
}
