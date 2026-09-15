package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
)

// Regression test. The root shim's self-bootstrap fallback pipes install.sh into bash. The shim
// must be safe to run as `eval "$(hermit shell-hooks --print --zsh)"`, so its stdout must stay
// clean; only stderr may carry install.sh's progress messages.
func TestGenInstallerSystemShimDoesNotLeakBootstrapOutputToStdout(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not available")
	}

	const noise1 = "Creating /fake/state/dir"
	const noise2 = "Downloading https://example.invalid/hermit.gz"

	// Fake install.sh: prints progress like the real one, then installs a stub $HERMIT_EXE.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `#!/bin/bash
set -euo pipefail
echo %q
echo %q
mkdir -p "$(dirname "$HERMIT_EXE")"
cat > "$HERMIT_EXE" <<'INNER'
#!/bin/bash
echo "REAL_HERMIT_INVOKED $*"
INNER
chmod +x "$HERMIT_EXE"
`, noise1, noise2)
	}))
	defer srv.Close()

	dir := t.TempDir()

	script, _, err := GenInstaller(Config{
		BaseDistURL:  srv.URL,
		InstallPaths: []string{dir},
	})
	assert.NoError(t, err)

	installerPath := filepath.Join(dir, "install.sh")
	assert.NoError(t, os.WriteFile(installerPath, script, 0o755))

	// Run system_install() only, to write the root shim into dir.
	installCmd := exec.Command("bash", installerPath)
	installCmd.Env = append(os.Environ(),
		"HERMIT_SKIP_USER_INSTALL=1",
		"HERMIT_BIN_INSTALL_DIR="+dir,
		"HERMIT_DIST_URL="+srv.URL,
	)
	out, err := installCmd.CombinedOutput()
	assert.NoError(t, err, "rendered install.sh failed: %s", out)

	shimPath := filepath.Join(dir, "hermit")
	_, err = os.Stat(shimPath)
	assert.NoError(t, err, "expected root shim at %s", shimPath)

	// HERMIT_EXE doesn't exist, so the shim must self-bootstrap. eval only sees stdout, so that's
	// the stream that must stay clean.
	fakeExe := filepath.Join(dir, "not-installed-yet", "hermit")
	shimCmd := exec.Command(shimPath, "some-arg")
	shimCmd.Env = append(os.Environ(), "HERMIT_EXE="+fakeExe)

	var stdout, stderr bytes.Buffer
	shimCmd.Stdout = &stdout
	shimCmd.Stderr = &stderr
	err = shimCmd.Run()
	assert.NoError(t, err, "shim failed, stderr: %s", stderr.String())

	assert.Equal(t, "REAL_HERMIT_INVOKED some-arg\n", stdout.String())
	assert.Contains(t, stderr.String(), noise1)
	assert.Contains(t, stderr.String(), noise2)
}
