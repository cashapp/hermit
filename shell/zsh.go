package shell

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const zshShellHooks = commonHooks + `
precmd_functions+=(change_hermit_env)
`

// Zsh represents the Zsh shell
type Zsh struct{ posixMixin }

var _ Shell = &Zsh{}

func (sh *Zsh) Name() string { return "zsh" } // nolint: golint

func (sh *Zsh) ActivationScript(w io.Writer, envName, root string) error { // nolint: golint
	err := posixActivationScriptTmpl.Execute(w, &posixActivationContext{
		Root:    root,
		EnvName: envName,
		Shell:   "zsh",
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
