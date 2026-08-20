//go:build integration

package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestFishActivationEscapesTrailingBackslash(t *testing.T) {
	fish, err := exec.LookPath("fish")
	assert.NoError(t, err)

	environ := buildAndInjectHermit(t, buildEnviron(t))
	stateDir := filepath.Join(t.TempDir(), "state")
	userConfigFile := filepath.Join(t.TempDir(), ".hermit.hcl")
	err = os.WriteFile(userConfigFile, nil, 0600)
	assert.NoError(t, err)
	environ = append(environ,
		"HERMIT_USER_CONFIG="+userConfigFile,
		"HERMIT_STATE_DIR="+stateDir,
	)

	var hermitExe string
	for _, entry := range environ {
		if strings.HasPrefix(entry, "HERMIT_EXE=") {
			hermitExe = strings.TrimPrefix(entry, "HERMIT_EXE=")
			break
		}
	}
	if hermitExe == "" {
		t.Fatal("HERMIT_EXE not found in integration test environment")
	}

	dir := t.TempDir()
	cmd := exec.Command(hermitExe, "init", "--no-git", ".")
	cmd.Dir = dir
	cmd.Env = environ
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "%s", output)

	// Regression test for DX-30: a trailing backslash must not escape the
	// closing quote and allow the following environment value to execute.
	config := `env = {
  "AAA": "\\",
  "AAB": ";touch RCE.txt; #",
}
`
	err = os.WriteFile(filepath.Join(dir, "bin", "hermit.hcl"), []byte(config), 0600)
	assert.NoError(t, err)

	cmd = exec.Command(fish, "--no-config", "-c", `
bin/hermit activate . | source
printf '%s' "$AAA" > AAA.txt
printf '%s' "$AAB" > AAB.txt
`)
	cmd.Dir = dir
	cmd.Env = environ
	output, err = cmd.CombinedOutput()
	assert.NoError(t, err, "%s", output)

	aaa, err := os.ReadFile(filepath.Join(dir, "AAA.txt"))
	assert.NoError(t, err)
	assert.Equal(t, `\`, string(aaa))
	aab, err := os.ReadFile(filepath.Join(dir, "AAB.txt"))
	assert.NoError(t, err)
	assert.Equal(t, `;touch RCE.txt; #`, string(aab))
	_, err = os.Stat(filepath.Join(dir, "RCE.txt"))
	assert.True(t, os.IsNotExist(err), "payload created RCE.txt")
}
