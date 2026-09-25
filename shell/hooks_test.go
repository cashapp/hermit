package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func TestChangeHermitEnvWalksParentsWithoutDirname(t *testing.T) {
	for _, name := range []string{"bash", "zsh"} {
		t.Run(name, func(t *testing.T) {
			executable, err := exec.LookPath(name)
			if err != nil {
				t.Skipf("%s is unavailable: %v", name, err)
			}

			root := t.TempDir()
			envRoot := filepath.Join(root, "project with spaces")
			assert.NoError(t, os.MkdirAll(filepath.Join(envRoot, "bin"), 0700))
			assert.NoError(t, os.MkdirAll(filepath.Join(envRoot, "nested", "deep"), 0700))
			assert.NoError(t, os.MkdirAll(filepath.Join(root, "outside", "deep"), 0700))
			assert.NoError(t, os.Symlink(envRoot, filepath.Join(root, "project-link")))
			assert.NoError(t, os.WriteFile(filepath.Join(envRoot, "bin", "activate-hermit"), []byte(`
HERMIT_ENV=$CUR
activations=$((activations + 1))
`), 0600))

			// Load just the shared hook so each shell exercises the same traversal.
			script := commonHooks + `
set -eu
unset HERMIT_ENV DEACTIVATED_HERMIT compstate
activations=0
refreshes=0
deactivations=0
HERMIT_ROOT_BIN=validate_env
validate_env() {
  test "$1" = --quiet && test "$2" = validate && test "$3" = env
}
update_hermit_env() { refreshes=$((refreshes + 1)); }
_hermit_deactivate() {
  unset HERMIT_ENV
  deactivations=$((deactivations + 1))
}
dirname() {
  printf 'unexpected dirname subprocess\n' >&2
  command dirname "$@"
}
check() {
  if ! "$@"; then
    printf 'assertion failed: %s\n' "$*" >&2
    exit 1
  fi
}

cd "$1/project with spaces/nested/deep"
change_hermit_env
check test "$HERMIT_ENV" -ef "$1/project with spaces"
check test "$activations" = 1

cd "$1/project-link/nested/deep"
change_hermit_env
check test "$HERMIT_ENV" -ef "$1/project with spaces"
check test "$activations" = 1
check test "$refreshes" = 1

cd "$1/outside/deep"
change_hermit_env
check test -z "${HERMIT_ENV-}"
check test "$deactivations" = 1

cd /
change_hermit_env
check test -z "${HERMIT_ENV-}"
check test "$deactivations" = 1
`
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-f", "-c", script, "hooks-test", root)
			output, err := cmd.CombinedOutput()
			assert.NoError(t, err, "%s", output)
			assert.Equal(t, "", string(output))
		})
	}
}
