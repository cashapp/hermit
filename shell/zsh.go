package shell

import (
	"io"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
)

var zshShellHooks = `
chpwd_functions+=(change_hermit_env)
change_hermit_env

# A child zsh can inherit HERMIT_ENV/ACTIVE_HERMIT from its parent without
# inheriting shell-local helpers like deactivate-hermit. Re-source
# activate-hermit so this shell rebuilds its Hermit functions and bookkeeping.
if [[ -n ${HERMIT_ENV+_} ]] && ! type deactivate-hermit >/dev/null 2>&1 && [[ -f "${HERMIT_ENV}/bin/activate-hermit" ]]; then
  unset ACTIVE_HERMIT HERMIT_ENV_OPS HERMIT_BIN_CHANGE
  . "${HERMIT_ENV}/bin/activate-hermit"
fi

# shellcheck disable=SC2154
if [[ -n ${_comps+x} ]]; then
  autoload -U +X bashcompinit && bashcompinit
  complete -o nospace -C "${HERMIT_ROOT_BIN:-"$HOME/bin/hermit"} noop" hermit
fi
`

// Zsh represents the Zsh shell
type Zsh struct{ posixMixin }

var _ Shell = &Zsh{}

func (sh *Zsh) Name() string { return "zsh" } // nolint: golint

func (sh *Zsh) ActivationScript(w io.Writer, config ActivationConfig) error { // nolint: golint
	err := posixActivationScriptTmpl.Execute(w, &posixActivationContext{
		EnvName:          filepath.Base(config.Root),
		ActivationConfig: config,
		Shell:            "zsh",
	})
	return errors.WithStack(err)
}

func (sh *Zsh) ActivationHooksInstallation() (path, script string, err error) { // nolint: golint
	return activationHooksInstallation(".zshrc", "zsh")
}

func (sh *Zsh) ActivationHooksCode() (script string, err error) { // nolint: golint
	return commonHooks + zshShellHooks, nil
}
