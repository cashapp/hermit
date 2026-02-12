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
