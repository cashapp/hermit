package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestActivationHooksInstallationFindsHermitOnPath(t *testing.T) {
	tests := []struct {
		name string
		sh   Shell
		args []string
	}{
		{name: "bash", sh: &Bash{}, args: []string{"-c"}},
		{name: "zsh", sh: &Zsh{}, args: []string{"-f", "-c"}},
		{name: "fish", sh: &Fish{}, args: []string{"-i", "-c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, script, err := tt.sh.ActivationHooksInstallation()
			assert.NoError(t, err)

			shellPath, err := exec.LookPath(tt.name)
			if err != nil {
				if tt.name == "bash" {
					t.Fatalf("bash is required to test POSIX shell hook installation: %v", err)
				}
				t.Skipf("%s is not installed", tt.name)
			}

			script += `
test "$HERMIT_ROOT_BIN" = "$EXPECTED_HERMIT" && test "$HERMIT_HOOK_LOADED" = 1
`

			hermitPath := writeFakeHermit(t)
			home := t.TempDir()
			cmd := exec.Command(shellPath, append(tt.args, script)...)
			cmd.Env = []string{
				"EXPECTED_HERMIT=" + hermitPath,
				"HOME=" + home,
				"PATH=" + filepath.Dir(hermitPath) + string(os.PathListSeparator) + os.Getenv("PATH"),
				"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
			}
			output, err := cmd.CombinedOutput()
			assert.NoError(t, err, "%s", output)
		})
	}
}

func writeFakeHermit(t *testing.T) string {
	t.Helper()

	binDir := filepath.Join(t.TempDir(), "homebrew bin")
	err := os.MkdirAll(binDir, 0700)
	assert.NoError(t, err)

	hermitPath := filepath.Join(binDir, "hermit")
	err = os.WriteFile(hermitPath, []byte(`#!/bin/sh
set -eu

test "$1" = shell-hooks
test "$2" = --print
case "$3" in
  --bash|--zsh) echo 'export HERMIT_HOOK_LOADED=1' ;;
  --fish) echo 'set -gx HERMIT_HOOK_LOADED 1' ;;
  *) exit 1 ;;
esac
`), 0700)
	assert.NoError(t, err)
	return hermitPath
}
