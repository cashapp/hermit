package shell

import (
	"io"

	"github.com/cashapp/hermit/errors"
)

var bashShellHooks = `
# A child bash inherits Hermit's variables but not its functions. Re-activate
# at the prompt so Hermit also overrides changes made by startup files.
restore_hermit_env() {
  if [ -z "${HERMIT_ENV+_}" ] || type deactivate-hermit >/dev/null 2>&1 || [ ! -f "${HERMIT_ENV}/bin/activate-hermit" ]; then
    return 0
  fi
  local env="${HERMIT_ENV}"
  if [ -n "${HERMIT_ENV_OPS-}" ]; then
    eval "$("${env}/bin/hermit" env --deactivate-from-ops="${HERMIT_ENV_OPS}")"
  fi
  unset ACTIVE_HERMIT HERMIT_ENV_OPS HERMIT_BIN_CHANGE
  . "${env}/bin/activate-hermit"
}

if test -n "${PROMPT_COMMAND+_}"; then
  PROMPT_COMMAND="restore_hermit_env; change_hermit_env; $PROMPT_COMMAND"
else
  PROMPT_COMMAND="restore_hermit_env; change_hermit_env"
fi

complete -o nospace -C "${HERMIT_ROOT_BIN:-"$HOME/bin/hermit"} noop" hermit
`

// Bash represent the Bash shell
type Bash struct{ posixMixin }

var _ Shell = &Bash{}

func (sh *Bash) Name() string { return "bash" }

func (sh *Bash) ActivationScript(w io.Writer, config ActivationConfig) error {
	err := posixActivationScriptTmpl.Execute(w, &posixActivationContext{
		ActivationConfig: config,
		EnvName:          envName(config.Root),
		Shell:            "bash",
	})
	return errors.WithStack(err)
}

func (sh *Bash) ActivationHooksInstallation() (path, script string, err error) {
	return activationHooksInstallation(".bashrc", "bash")
}

func (sh *Bash) ActivationHooksCode() (script string, err error) {
	return commonHooks + bashShellHooks, nil
}
