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

// initFishEnv initialises a Hermit environment with a freshly built Hermit,
// returning the fish executable, the environment directory, and its environ.
func initFishEnv(t *testing.T) (fish, dir string, environ []string) {
	t.Helper()
	fish, err := exec.LookPath("fish")
	assert.NoError(t, err)

	environ = buildAndInjectHermit(t, buildEnviron(t))
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

	dir = t.TempDir()
	cmd := exec.Command(hermitExe, "init", "--no-git", ".")
	cmd.Dir = dir
	cmd.Env = environ
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "%s", output)
	return fish, dir, environ
}

func TestFishActivationEscapesTrailingBackslash(t *testing.T) {
	fish, dir, environ := initFishEnv(t)

	// Regression test for DX-30: a trailing backslash must not escape the
	// closing quote and allow the following environment value to execute.
	config := `env = {
  "AAA": "\\",
  "AAB": ";touch RCE.txt; #",
}
`
	err := os.WriteFile(filepath.Join(dir, "bin", "hermit.hcl"), []byte(config), 0600)
	assert.NoError(t, err)

	cmd := exec.Command(fish, "--no-config", "-c", `
bin/hermit activate . | source
printf '%s' "$AAA" > AAA.txt
printf '%s' "$AAB" > AAB.txt
`)
	cmd.Dir = dir
	cmd.Env = environ
	output, err := cmd.CombinedOutput()
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

func TestFishPromptHookAppliesInPlaceConfigEdit(t *testing.T) {
	fish, dir, environ := initFishEnv(t)

	// Backdate the environment so the hook's second-resolution mtime check sees
	// the edit, and rewrite hermit.hcl in place so the mtime of bin is unchanged.
	cmd := exec.Command(fish, "--no-config", "-c", `
touch -t 200001010000 bin bin/hermit.hcl
bin/hermit activate . | source
printf 'env = { "FOO": "bar" }\n' > bin/hermit.hcl
emit fish_prompt
printf '%s' "$FOO" > FOO.txt
`)
	cmd.Dir = dir
	cmd.Env = environ
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "%s", output)

	foo, err := os.ReadFile(filepath.Join(dir, "FOO.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "bar", string(foo))
}

func TestFishReassertsEnvironmentAtFirstPrompt(t *testing.T) {
	fish, dir, environ := initFishEnv(t)

	config := `env = {
  "PATH": "${HERMIT_ENV}/tool/bin:${PATH}",
  "TOOL_HOME": "${HERMIT_ENV}/tool",
}
`
	err := os.WriteFile(filepath.Join(dir, "bin", "hermit.hcl"), []byte(config), 0600)
	assert.NoError(t, err)

	// Hermit's conf.d hook activates before config.fish, which may then change
	// the environment.
	cmd := exec.Command(fish, "--no-config", "-c", `
bin/hermit activate . | source
set -gx PATH /rvm/bin $PATH
set -gx TOOL_HOME /rvm/tool
emit fish_prompt
string join : $PATH > reasserted-PATH.txt
printf '%s' "$TOOL_HOME" > reasserted-TOOL_HOME.txt
deactivate-hermit
string join : $PATH > deactivated-PATH.txt
printf '%s' "$TOOL_HOME" > deactivated-TOOL_HOME.txt
`)
	cmd.Dir = dir
	cmd.Env = environ
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "%s", output)

	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		assert.NoError(t, err)
		return string(data)
	}
	root, err := filepath.EvalSymlinks(dir)
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(read("reasserted-PATH.txt"), root+"/tool/bin:"), "%s", read("reasserted-PATH.txt"))
	assert.Contains(t, read("reasserted-PATH.txt"), ":"+root+"/bin:/rvm/bin:")
	assert.Equal(t, root+"/tool", read("reasserted-TOOL_HOME.txt"))
	// Deactivation restores what config.fish set.
	assert.True(t, strings.HasPrefix(read("deactivated-PATH.txt"), "/rvm/bin:"), "%s", read("deactivated-PATH.txt"))
	assert.Equal(t, "/rvm/tool", read("deactivated-TOOL_HOME.txt"))
}
