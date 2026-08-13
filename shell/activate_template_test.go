package shell

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/envars"
)

func TestPosixActivationScriptQuotesPathsWithSpaces(t *testing.T) {
	root := "/tmp/Application Support/hermit env"

	var out bytes.Buffer
	err := (&Bash{}).ActivationScript(&out, ActivationConfig{
		Root:   root,
		Prompt: "none",
		Env: envars.Envars{
			"HERMIT_BIN": root + "/bin",
		},
	})
	assert.NoError(t, err)

	script := out.String()
	assert.Contains(t, script, "export HERMIT_ENV='/tmp/Application Support/hermit env'")
	assert.Contains(t, script, `export ACTIVE_HERMIT="${HERMIT_ENV}"`)
	assert.Contains(t, script, `eval "$("${ACTIVE_HERMIT}/bin/hermit" env --deactivate-from-ops="${HERMIT_ENV_OPS}")"`)
	assert.Contains(t, script, `echo "Hermit environment $("${HERMIT_ENV}/bin/hermit" env HERMIT_ENV) deactivated"`)
	assert.Contains(t, script, `export HERMIT_ENV_OPS="$("${HERMIT_ENV}/bin/hermit" env --ops)"`)
	assert.Contains(t, script, `export HERMIT_BIN_CHANGE="$(date -r "${HERMIT_ENV}/bin" +"%s")"`)
	assert.Contains(t, script, `local CUR_HERMIT="${HERMIT_ENV}/bin/hermit"`)

	if strings.Contains(script, "export HERMIT_ENV=/tmp/Application Support/hermit env") {
		t.Fatalf("generated script still contains unquoted HERMIT_ENV assignment:\n%s", script)
	}
}

func TestActivateHermitRejectsMaliciousEnvKey(t *testing.T) {
	// Regression test for VULN-78160: shell command injection via env-var key.
	maliciousEnv := envars.Envars{
		"EVIL; touch /tmp/pwned; X": "innocent",
	}
	cases := []Shell{&Bash{}, &Zsh{}, &Fish{}}
	for _, sh := range cases {
		t.Run(sh.Name(), func(t *testing.T) {
			var out bytes.Buffer
			err := ActivateHermit(&out, sh, ActivationConfig{
				Root:   "/tmp/env",
				Prompt: "none",
				Env:    maliciousEnv,
			})
			assert.Error(t, err)

			out.Reset()
			err = sh.ApplyEnvars(&out, maliciousEnv)
			assert.Error(t, err)
		})
	}
}

func TestActivationScriptNeutralisesMaliciousEnvBasename(t *testing.T) {
	// Regression test for VULN-78225: command injection via the environment basename.
	payload := `$(id>RCE_ID.txt;whoami>RCE_WHOAMI.txt;printf PWNED>RCE_WRITE.txt)`
	for _, sh := range []Shell{&Bash{}, &Zsh{}, &Fish{}} {
		t.Run(sh.Name(), func(t *testing.T) {
			var out bytes.Buffer
			err := ActivateHermit(&out, sh, ActivationConfig{
				Root:   "/tmp/" + payload,
				Prompt: "env",
				Env:    envars.Envars{"HERMIT_BIN": "/tmp/" + payload + "/bin"},
			})
			assert.NoError(t, err)

			script := out.String()
			checked := 0
			for line := range strings.SplitSeq(script, "\n") {
				_, assignment, ok := strings.Cut(line, `PS1="`)
				if !ok {
					continue
				}
				name, _, ok := strings.Cut(assignment, "🐚")
				if !ok {
					continue
				}
				for _, metachar := range []string{"$", "`", `"`, `\`, "%", "!", ";", ">"} {
					assert.NotContains(t, name, metachar, "%s", line)
				}
				checked++
			}
			// Fish does not interpolate the environment name into its prompt.
			if sh.Name() != "fish" {
				assert.True(t, checked > 0, "no prompt assignment found in:\n%s", script)
			}
			assert.NotContains(t, script, `PS1="$(`)
			assert.Contains(t, script, "'/tmp/"+payload+"'")
		})
	}
}

func TestFishActivationScriptQuotesPathsWithSpaces(t *testing.T) {
	root := "/tmp/Application Support/hermit env"

	var out bytes.Buffer
	err := (&Fish{}).ActivationScript(&out, ActivationConfig{
		Root:   root,
		Prompt: "none",
		Env: envars.Envars{
			"HERMIT_BIN": root + "/bin",
		},
	})
	assert.NoError(t, err)

	script := out.String()
	assert.Contains(t, script, "set -gx HERMIT_ENV '/tmp/Application Support/hermit env'")
	assert.Contains(t, script, `set -gx ACTIVE_HERMIT "$HERMIT_ENV"`)
	assert.Contains(t, script, `echo "Hermit environment $("$HERMIT_ENV/bin/hermit" env HERMIT_ENV) deactivated"`)
	assert.Contains(t, script, `set -gx HERMIT_ENV_OPS "$("$HERMIT_ENV/bin/hermit" env --ops)"`)

	if strings.Contains(script, "set -gx HERMIT_ENV /tmp/Application Support/hermit env") {
		t.Fatalf("generated fish script still contains unquoted HERMIT_ENV assignment:\n%s", script)
	}
}

func TestFishEscapesBackslashesInValues(t *testing.T) {
	config := ActivationConfig{
		Root:   `/tmp/hermit\`,
		Prompt: "none",
		Env: envars.Envars{
			"AAA": `\`,
			"AAB": `;touch /tmp/pwned; #`,
		},
	}

	var out bytes.Buffer
	err := (&Fish{}).ActivationScript(&out, config)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), `set -gx HERMIT_ENV '/tmp/hermit\\'`)
	assert.Contains(t, out.String(), `set -gx AAA '\\'`)
	assert.Contains(t, out.String(), `set -gx AAB ';touch /tmp/pwned; #'`)
	assert.NotContains(t, out.String(), `set -gx AAA '\'`)

	out.Reset()
	err = (&Fish{}).ApplyEnvars(&out, config.Env)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), `set -gx AAA '\\'`)
	assert.NotContains(t, out.String(), `set -gx AAA '\'`)
}
