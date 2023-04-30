package shell

import (
	"io"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
)

var zshShellHooks = `
precmd_functions+=(change_hermit_env)

# shellcheck disable=SC2154
if [[ -n ${_comps+x} ]]; then
  autoload -U +X bashcompinit && bashcompinit
  complete -o nospace -C "${HERMIT_ROOT_BIN:-"$HOME/bin/hermit"}" hermit
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
