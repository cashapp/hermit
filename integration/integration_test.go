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
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/creack/pty"
)

var shells = [][]string{
	{"bash", "--norc", "--noprofile"},
	{"zsh", "--no-rcs", "--no-globalrcs"},
}

// Functions that test scripts can use to communicate back to the test framework.
const preamble = `
set -euo pipefail

hermit-send() {
  echo "$@" 1>&3
}

assert() {
  if ! "$@"; then
    hermit-send "error: assertion failed: $@"
    exit 1
  fi
}

# Run a shell command and emulate what the Hermit shell hooks would do.
#
# usage: with-prompt-hooks <cmd>
#
# Normally this is done by shell hooks, but because we're not running interactively this is not possible.
with-prompt-hooks() {
  "$@"
  res=$?
  # We need to reset the change timestamp, as file timestamps are at second resolution.
  # Some IT updates could be lost without this
  export HERMIT_BIN_CHANGE=0

  if test -n "${PROMPT_COMMAND+_}"; then
    eval "$PROMPT_COMMAND"
  elif [ -n "${ZSH_VERSION-}" ]; then
    update_hermit_env
  fi

  return $res
}
`

func TestIntegration(t *testing.T) {
	tests := []struct {
		name         string
		preparations prep
		script       string
		fails        bool // The script will exit with a non-zero exit status.
		expectations exp
	}{
		{
			name: "UsingCustomHermit",
			script: `
				which hermit
				hermit --version
			`,
			expectations: exp{outputContains("/hermit-local/hermit"), outputContains("devel (canary)")},
		},
		{
			name: "Init",
			script: `
				hermit init --idea .
			`,
			expectations: exp{
				filesExist(
					"bin/hermit", ".idea/externalDependencies.xml",
					"bin/activate-hermit", "bin/hermit.hcl"),
				outputContains("Creating new Hermit environment"),
			},
		},
		{
			name: "InitWithUserConfigDefaults",
			script: `
				cat > "$HERMIT_USER_CONFIG" <<EOF
defaults {
	sources = ["source1", "source2"]
	manage-git = false
	idea = true
}
EOF
				hermit init .
				echo "Generated bin/hermit.hcl content:"
				cat bin/hermit.hcl
			`,
			expectations: exp{
				filesExist("bin/hermit.hcl"),
				fileContains("bin/hermit.hcl", `sources = \["source1", "source2"\]`),
				fileContains("bin/hermit.hcl", `manage-git = false`),
				fileContains("bin/hermit.hcl", `idea = true`),
			},
		},
		{
			name: "InitWithUserConfigTopLevelNoGitAndIdeaOverrideDefaults",
			script: `
				cat > "$HERMIT_USER_CONFIG" <<EOF
no-git = true
idea = true
defaults {
	manage-git = true
	idea = false
}
EOF
				hermit init .
				echo "Generated bin/hermit.hcl content:"
				cat bin/hermit.hcl
			`,
			expectations: exp{
				filesExist("bin/hermit.hcl"),
				fileContains("bin/hermit.hcl", `manage-git = false`),
				fileContains("bin/hermit.hcl", `idea = true`),
			},
		},
		{
			name: "InitWithCommandLineOverridesDefaults",
			script: `
				cat > "$HERMIT_USER_CONFIG" <<EOF
defaults {
	sources = ["source1", "source2"]
	manage-git = false
	idea = false
}
EOF
				hermit init --sources source3,source4 --idea .
			`,
			expectations: exp{
				filesExist("bin/hermit.hcl"),
				fileContains("bin/hermit.hcl", `sources = \["source3", "source4"\]`),
				fileContains("bin/hermit.hcl", `manage-git = false`),
				fileContains("bin/hermit.hcl", `idea = true`),
			},
		},
		{
			name: "InitSourcesCommandLineOverridesDefaults",
			script: `
				cat > "$HERMIT_USER_CONFIG" <<EOF
defaults {
	sources = ["source1", "source2"]
}
EOF
				hermit init --sources source3,source4 .
				cat bin/hermit.hcl
			`,
			expectations: exp{
				filesExist("bin/hermit.hcl"),
				fileContains("bin/hermit.hcl", `sources = \["source3", "source4"\]`),
				fileDoesNotContain("bin/hermit.hcl", `\["source1"`),
				fileDoesNotContain("bin/hermit.hcl", `\["source2"`),
			},
		},
		{
			name: "HermitEnvarIsSet",
			script: `
				hermit init .
				. bin/activate-hermit
				assert test -n "$HERMIT_ENV"
			`,
		},
		{
			name: "CannotBeActivatedTwice",
			script: `
				hermit init .
				. bin/activate-hermit
				. bin/activate-hermit
			`,
			expectations: exp{outputContains("This Hermit environment has already been activated. Skipping")},
		},
		{
			name:         "PackageEnvarsAreSetAutomatically",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				assert test -z "$(hermit env FOO)"
				with-prompt-hooks hermit install testbin1
				assert test "${FOO:-}" = "bar"
			`,
		},
		{
			name:         "HermitEnvCommandSetsAutomatically",
			preparations: prep{fixture("testenv1")},
			script: `
				hermit init .
				. bin/activate-hermit
				assert test -z "$(hermit env FOO)"
				with-prompt-hooks hermit env FOO bar
				assert test "${FOO:-}" = "bar"
			`,
		},
		{
			name: "EnvEnvarsAreSetDuringActivation",
			script: `
				assert test "${BAR:-}" = "waz"
			`,
			preparations: prep{fixture("testenv1"), activate(".")},
		},
		{
			name:         "InstallingPackageCreatesSymlinks",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				assert test ! -L bin/testbin1

				hermit install testbin1-1.0.1
				assert test "$(readlink bin/testbin1)" = ".testbin1-1.0.1.pkg"
				assert test "$(readlink bin/.testbin1-1.0.1.pkg)" = "hermit"

				hermit install testbin1-1.0.0
				assert test "$(readlink bin/testbin1)" = ".testbin1-1.0.0.pkg"
				assert test "$(readlink bin/.testbin1-1.0.0.pkg)" = "hermit"
			`,
		},
		{
			name:         "UninstallingRemovesSymlinks",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				hermit install testbin1-1.0.1
				assert test "$(readlink bin/testbin1)" = ".testbin1-1.0.1.pkg"
				hermit uninstall testbin1
				assert test ! -L bin/testbin1
			`,
		},
		{
			name:         "DowngradingPackageWorks",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				hermit install testbin1-1.0.1
				assert test "$(testbin1)" = "testbin1 1.0.1"
				hermit install testbin1-1.0.0
				assert test "$(testbin1)" = "testbin1 1.0.0"
			`,
		},
		{
			name:         "UpgradingPackageWorks",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				hermit install testbin1-1.0.0
				assert test "$(testbin1)" = "testbin1 1.0.0"
				hermit upgrade testbin1
				assert test "$(testbin1)" = "testbin1 1.0.1"
			`,
		},
		{
			name:         "InstallingPackageSetsEnvarsInShell",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				with-prompt-hooks hermit install testbin1-1.0.0
				assert test "${TESTBIN1VERSION:-}" = "1.0.0"

				with-prompt-hooks hermit install testbin1-1.0.1
				assert test "${TESTBIN1VERSION:-}" = "1.0.1"
			`,
		},
		{
			name:         "InstallingEnvPackagesIsaNoop",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				hermit install
			`,
		},
		{
			name:         "DeactivatingRemovesHermitEnvars",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
				deactivate-hermit
				assert test -z "${HERMIT_ENV:-}"
			`,
		},
		{
			name:         "DeactivatingRestoresEnvars",
			preparations: prep{fixture("testenv1")},
			script: `
				export BAR="foo"
				. bin/activate-hermit
				assert test "${BAR:-}" = "waz"
				deactivate-hermit
				assert test "${BAR:-}" = "foo"
			`,
		},
		{
			name:         "SwitchingEnvironmentsWorks",
			preparations: prep{allFixtures("testenv1", "testenv2")},
			script: `
				. testenv1/bin/activate-hermit
				assert test "${HERMIT_ENV:-}" = "$PWD/testenv1"
				. testenv2/bin/activate-hermit
				assert test "${HERMIT_ENV:-}" = "$PWD/testenv2"
			`,
		},
		{
			name:         "ExecuteFromAnotherEnvironmentWorks",
			preparations: prep{allFixtures("testenv1", "testenv2")},
			script: `
				testenv2/bin/hermit install testbin1
				. testenv1/bin/activate-hermit
				assert test "$(./testenv2/bin/testbin1)" = "testbin1 1.0.1"
			`,
		},
		{
			name:         "ExecuteFromOtherEnvironmentLoadsDependentEnvars",
			preparations: prep{allFixtures("testenv1", "testenv2")},
			// Due to https://github.com/cashapp/hermit/issues/203 testbin4 and testbin1
			// must be installed separately.
			script: `
				testenv2/bin/hermit install testbin4
				testenv2/bin/hermit install testbin1-1.0.0
				. testenv1/bin/activate-hermit
				hermit install testbin1-1.0.1
				assert test "$(./testenv2/bin/testbin4)" = "env[1.0.0] exec[testbin1 1.0.0]"
			`,
		},
		{
			name:         "StubFromOtherEnvironmentHasItsOwnEnvars",
			preparations: prep{allFixtures("testenv1", "testenv2")},
			script: `
				testenv2/bin/hermit install testbin1
				. testenv1/bin/activate-hermit
				assert test "$(testenv2/bin/hermit env TESTENV2)" = "yes"
			`,
		},
		{
			name:         "InstallDirectScriptPackage",
			preparations: prep{fixture("testenv2"), activate(".")},
			script: `
				hermit install testbin2
				assert test "$(testbin2)" = "testbin2 2.0.1"
			`,
		},
		{
			name:         "InstallNonExecutablePackage",
			preparations: prep{fixture("testenv2"), activate(".")},
			script: `
				hermit install testbin3
				assert test "$(testbin3)" = "testbin3 3.0.1"
			`,
		},
		{
			name:         "AddDigests",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
			hermit manifest add-digests packages/testbin1.hcl
			assert grep d4f8989a4a6bf56ccc768c094448aa5f42be3b9f0287adc2f4dfd2241f80d2c0 packages/testbin1.hcl
			`,
		},
		{
			name:         "UpgradeTriggersInstallHook",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
			hermit install testbin1-1.0.0
			hermit upgrade testbin1
			`,
			expectations: exp{outputContains("testbin1-1.0.0 hook"), outputContains("testbin1-1.0.1 hook")},
		},
		{
			name:         "SymlinkAndMkdirActionsWork",
			preparations: prep{fixture("testenv3"), activate(".")},
			script: `
			hermit install testbin1
			testbin1
			testbin2
			assert test -d exec-hook-triggered
			`,
			expectations: exp{outputContains("testbin1 1.0.1")},
		},
		{
			name:         "RuntimeDepsEnvOverridesUnrelatedPackageEnv",
			preparations: prep{fixture("testenv4"), activate(".")},
			script: `
			hermit install testbin1
			hermit install testbin2
			hermit install other
			assert test "$(testbin1.sh)" = "FOO=runtimefoo"
			assert test "$(testbin2.sh)" = "BAR=hermitbar"
			`,
		},
		{
			name:         "EnvironmentsWithOverlappingEnvVariablesCanBeSwitched",
			preparations: prep{fixture("overlapping_envs")},
			script: `
			. env1/bin/activate-hermit
			assert test "$FOO" = "BAR"

			. env2/bin/activate-hermit
			assert test "$FOO" = "BAR"
			`,
		},
		{
			name:         "InheritsEnvVariables",
			preparations: prep{fixture("environment_inheritance"), activate("child_environment")},
			script:       `echo "${OVERWRITTEN} - ${NOT_OVERWRITTEN}"`,
			expectations: exp{outputContains("child - parent")},
		},
		{
			name:         "InheritanceOverwritesBinaries",
			preparations: prep{fixture("environment_inheritance"), activate(".")},
			script: `
			hermit install binary
			. child_environment/bin/activate-hermit
			assert test "$(binary.sh)" = "Running from parent"

			hermit install binary
			assert test "$(binary.sh)" = "Running from child"
			`,
		},
		{
			name:         "IndirectExecutionFromParentEnvLoadsCorrectEnvVars",
			preparations: prep{fixture("environment_inheritance"), activate(".")},
			// This test covers the nested environment case wherein:
			//  - a binary executed in a parent environment relies on environment
			//    variables set by a separate package Foo
			//  - package Foo is provided by both the parent and child
			//    environments
			script: `
			hermit install envprovider-1.0.0
			hermit install parentbin

      # Install before activating because automatic env var updates
      # only work when at least one second has passed since the HERMIT_ENV
      # dir was modified.
      ./child_environment/bin/hermit install envprovider-1.0.1
			. child_environment/bin/activate-hermit

      assert test "$VARIABLE" = "1.0.1"
      assert test "$(parentbin.sh)" = "parentenv: envprovider-1.0.0"
			`,
		},
		{
			name:         "MissingActivateFishShellFileIsValidEnv",
			preparations: prep{fixture("testenv1"), activate(".")},
			script: `
            hermit validate env .
`,
		},
	}

	checkForShells(t)
	environ := buildEnviron(t)
	environ = buildAndInjectHermit(t, environ)

	debug := os.Getenv("DEBUG") != ""

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, shell := range shells {
				t.Run(shell[0], func(t *testing.T) {
					stateDir := filepath.Join(t.TempDir(), "state")
					// Ensure the state dir is writeable so it can be deleted.
					t.Cleanup(func() {
						_ = filepath.Walk(stateDir, func(path string, info fs.FileInfo, err error) error {
							if err != nil {
								return err
							}
							if info.IsDir() {
								_ = os.Chmod(path, 0700)
							} else {
								_ = os.Chmod(path, 0600)
							}
							return nil
						})
					})
					dir := filepath.Join(t.TempDir(), "root")
					err := os.MkdirAll(dir, 0700)
					assert.NoError(t, err)
					testEnvars := make([]string, len(environ), len(environ)+2)
					copy(testEnvars, environ)

					// Create a unique empty config file for each test
					userConfigFile := filepath.Join(t.TempDir(), ".hermit.hcl")
					err = os.WriteFile(userConfigFile, []byte(""), 0600)
					assert.NoError(t, err)
					testEnvars = append(testEnvars, "HERMIT_USER_CONFIG="+userConfigFile)
					testEnvars = append(testEnvars, "HERMIT_STATE_DIR="+stateDir)

					prepScript := ""
					for _, prep := range test.preparations {
						fragment := prep(t, dir)
						if fragment != "" {
							prepScript += fragment + "\n"
						}
					}

					// FD 3 is used for the control protocol, allowing the test
					// scripts to communicate with the test harness.
					ctrlr, ctrlw, err := os.Pipe()
					assert.NoError(t, err)
					defer ctrlr.Close()
					defer ctrlw.Close()

					// Read lines of commands from FD 3 and send them to the control channel.
					controlch := make(chan string, 128)
					go func() {
						buf := bufio.NewScanner(ctrlr)
						for buf.Scan() {
							controlch <- buf.Text()
						}
						close(controlch)
					}()

					output := &strings.Builder{}
					var tee io.Writer = output
					if debug {
						tee = io.MultiWriter(output, os.Stderr)
					}

					script := preamble + "\n" + prepScript + "\n" + test.script
					args := append(shell[1:], "-c", script)
					cmd := exec.Command(shell[0], args...)
					cmd.Dir = dir
					cmd.Env = testEnvars
					cmd.ExtraFiles = []*os.File{ctrlw}

					// Start the test script.
					f, err := pty.Start(cmd)
					assert.NoError(t, err)
					defer f.Close()

					//nolint: errcheck // an io error is always returned, ignore it: read /dev/ptmx: input/output error
					io.Copy(tee, f)

					err = cmd.Wait()

					// Close the control FD and apply commands.
					_ = ctrlw.Close()
					for cmd := range controlch {
						parts := strings.SplitN(cmd, ":", 2)
						assert.Equal(t, 2, len(parts), "expected command to be in the form 'command:args'")
						switch parts[0] {
						case "error":
							t.Log(output.String())
							t.Fatal(parts[1])

						default:
							t.Log(output.String())
							t.Fatalf("unknown command: %s", parts[0])
						}
					}
					if test.fails {
						assert.Error(t, err, "%s", output.String())
					} else {
						assert.NoError(t, err, "%s", output.String())
					}
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
		_, err := exec.LookPath(shell[0])
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
	environ = append(environ, "PS1=hermit-test> ")
	environ = append(environ, "PROMPT=hermit-test> ")
	return environ
}

// Preparation applied to the test directory before running the test.
//
// Extra setup script fragments can be returned.
type (
	preparation func(t *testing.T, dir string) string
	prep        []preparation
)

func addFile(name, content string) preparation {
	return func(t *testing.T, dir string) string {
		t.Helper()
		err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600)
		assert.NoError(t, err)
		return ""
	}
}

// Copy a file from the testdata directory to the test directory.
func copyFile(name string) preparation {
	return func(t *testing.T, dir string) string {
		t.Helper()
		r, err := os.Open(filepath.Join("testdata", name))
		assert.NoError(t, err)
		defer r.Close()
		w, err := os.Create(filepath.Join(dir, name))
		assert.NoError(t, err)
		defer w.Close()
		_, err = io.Copy(w, r)
		assert.NoError(t, err)
		return ""
	}
}

// Copy the specified fixture directory under testdata into the test directory root.
func fixture(fixture string) preparation {
	return fixtureToDir(fixture, ".")
}

// Activate the Hermit environment relative to the test directory.
func activate(relDest string) preparation {
	return func(t *testing.T, dir string) string {
		return fmt.Sprintf(". %s/bin/activate-hermit", relDest)
	}
}

// Copy the specified environment fixture into the test root and activate it.
func activatedFixtureEnv(env string) preparation {
	return func(t *testing.T, dir string) string {
		fixture(env)(t, dir)
		return ". bin/activate-hermit"
	}
}

// Copy all the given fixtures to subdirectories of the test directory.
func allFixtures(fixtures ...string) preparation {
	return func(t *testing.T, dir string) string {
		for _, f := range fixtures {
			fixtureToDir(f, f)(t, dir)
		}
		return ""
	}
}

// Recursively copy a directory from the testdata directory to the test directory.
func fixtureToDir(relSource string, relDest string) preparation {
	return func(t *testing.T, dir string) string {
		source := filepath.Join("testdata", relSource)
		err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			dest := filepath.Join(dir, relDest, path[len(source):])
			if info.IsDir() {
				return os.MkdirAll(dest, 0700)
			}
			r, err := os.Open(path)
			if err != nil {
				return errors.Wrap(err, "failed to open source")
			}
			defer r.Close()
			w, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, info.Mode())
			if err != nil {
				return errors.Wrap(err, "failed to create dest")
			}
			defer w.Close()
			_, err = io.Copy(w, r)
			if err != nil {
				return errors.Wrap(err, "failed to copy")
			}
			return nil
		})
		assert.NoError(t, err)
		return ""
	}
}

// An expectation that must be met after running a test.
type (
	expectation func(t *testing.T, dir, stdout string)
	exp         []expectation
)

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

// Verify that the file under the test directory does not contain the given content.
func fileDoesNotContain(path, regex string) expectation {
	return func(t *testing.T, dir, stdout string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, path))
		assert.NoError(t, err)
		assert.False(t, regexp.MustCompile(regex).Match(data))
	}
}

// Verify that the output of the test script contains the given text.
func outputContains(text string) expectation {
	return func(t *testing.T, dir, output string) {
		t.Helper()
		assert.Contains(t, output, text, "%s", output)
	}
}
