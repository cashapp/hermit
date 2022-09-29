// Package integration provides integration tests for Hermit.
//
// Each test is run against the supported shells, in a temporary directory, with
// a version of Hermit built from the current source.
//
// Each test may provide a set of preparations, which are run before the test
// script, and a set of expectations that must be met, which are run after the
// test script.

//go:build integration

// nolint: deadcode
package integration_test

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/envars"
	"github.com/creack/pty"
)

var shells = []string{"bash", "zsh"}

func TestIntegration(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		preparations prep
		expectations exp
	}{
		{name: "UsingCustomHermit",
			script: `
				which hermit
				hermit --version
			`,
			expectations: exp{outputContains("/hermit-local/hermit"), outputContains("devel (canary)")}},
		{name: "Init",
			script: `
				hermit init --idea .
			`,
			expectations: exp{
				filesExist("bin/hermit", ".idea/externalDependencies.xml",
					"bin/activate-hermit", "bin/hermit.hcl"),
				outputContains("Creating new Hermit environment")}},
		{name: "HERMIT_ENV_IsSet",
			script: `
				hermit init .
				. bin/activate-hermit
				test -n "$HERMIT_ENV"
			`},
	}

	checkForShells(t)
	environ := buildEnviron(t)
	environ = buildAndInjectHermit(t, environ)

	debug := os.Getenv("DEBUG") != ""

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, shell := range shells {
				t.Run(shell, func(t *testing.T) {
					dir := t.TempDir()
					for _, prep := range test.preparations {
						prep(t, dir)
					}

					output := &strings.Builder{}
					cmd := exec.Command(shell, "-c", `set -euo pipefail;`+test.script)
					cmd.Dir = dir
					cmd.Env = environ
					f, err := pty.Start(cmd)
					assert.NoError(t, err)
					defer f.Close()

					go io.Copy(output, f)

					err = cmd.Wait()
					assert.NoError(t, err)

					if debug {
						t.Logf("Output:\n%s", output)
					}

					for _, expectation := range test.expectations {
						expectation(t, dir, output.String())
					}
				})
			}
		})
	}
}

// Build Hermit from source.
func buildAndInjectHermit(t *testing.T, environ []string) (outenviron []string) {
	t.Helper()
	dir := t.TempDir()
	hermitExeDir := filepath.Join(dir, "hermit-local")
	hermitExe := filepath.Join(hermitExeDir, "hermit")
	err := os.Mkdir(hermitExeDir, 0700)
	assert.NoError(t, err)
	t.Logf("Compiling Hermit to %s", hermitExe)
	output, err := exec.Command("go", "build", "-o", hermitExe, "github.com/cashapp/hermit/cmd/hermit").CombinedOutput()
	assert.NoError(t, err, "%s", output)
	outenviron = make([]string, len(environ), len(environ)+1)
	copy(outenviron, environ)
	outenviron = append(outenviron, "HERMIT_EXE="+hermitExe)
	for i, env := range outenviron {
		if strings.HasPrefix(env, "PATH=") {
			outenviron[i] = "PATH=" + hermitExeDir + ":" + env[len("PATH="):]
		}
	}
	return outenviron
}

func checkForShells(t *testing.T) {
	t.Helper()
	for _, shell := range shells {
		_, err := exec.LookPath(shell)
		assert.NoError(t, err)
	}
}

// Build a clean environment for the tests, removing Hermit env changes if necessary.
func buildEnviron(t *testing.T) (environ []string) {
	t.Helper()
	if os.Getenv("HERMIT_ENV_OPS") != "" {
		// Revert Hermit environment variables to their original values.
		ops, err := envars.UnmarshalOps([]byte(os.Getenv("HERMIT_ENV_OPS")))
		assert.NoError(t, err)
		ops = append(ops, &envars.Set{Name: "ACTIVE_HERMIT", Value: os.Getenv("ACTIVE_HERMIT")})
		environ = envars.Parse(os.Environ()).Revert(os.Getenv("HERMIT_ENV"), ops).Combined().System()
	} else {
		environ = os.Environ()
	}
	return environ
}

// Preparation applied to the test directory before running the test.
type preparation func(t *testing.T, dir string)
type prep []preparation

func addFile(name, content string) preparation {
	return func(t *testing.T, dir string) {
		t.Helper()
		err := ioutil.WriteFile(filepath.Join(dir, name), []byte(content), 0600)
		assert.NoError(t, err)
	}
}

// Copy a file from the testdata directory to the test directory.
func copyFile(name string) preparation {
	return func(t *testing.T, dir string) {
		t.Helper()
		r, err := os.Open(filepath.Join("testdata", name))
		assert.NoError(t, err)
		defer r.Close()
		w, err := os.Create(filepath.Join(dir, name))
		assert.NoError(t, err)
		defer w.Close()
		_, err = io.Copy(w, r)
		assert.NoError(t, err)
	}
}

// An expectation that must be met after running a test.
type expectation func(t *testing.T, dir, stdout string)
type exp []expectation

// Verify that the given paths exist in the test directory.
func filesExist(paths ...string) expectation {
	return func(t *testing.T, dir, stdout string) {
		t.Helper()
		for _, path := range paths {
			_, err := os.Stat(filepath.Join(dir, path))
			assert.NoError(t, err)
		}
	}
}

// Verify that the file under the test directory contains the given content.
func fileContains(path, regex string) expectation {
	return func(t *testing.T, dir, stdout string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, path))
		assert.NoError(t, err)
		assert.True(t, regexp.MustCompile(regex).Match(data))
	}
}

// Verify that the output of the test script contains the given text.
func outputContains(text string) expectation {
	return func(t *testing.T, dir, output string) {
		t.Helper()
		assert.Contains(t, output, text, "%s", output)
	}
}
