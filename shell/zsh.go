package shell

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const zshShellHooks = commonHooks + `
precmd_functions+=(change_hermit_env)

if [ ! -z ${_comps+x} ]; then
  autoload -U +X bashcompinit && bashcompinit
  complete -o nospace -C $HOME/bin/hermit hermit
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	fileName := filepath.Join(home, ".zshrc")
	return fileName, `eval "$(test -x $HOME/bin/hermit && $HOME/bin/hermit shell-hooks --print --zsh)"`, nil
}

func (sh *Zsh) ActivationHooksCode() (script string, err error) { // nolint: golint
	return zshShellHooks, err
}
