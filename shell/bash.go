package shell

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const bashShellHooks = commonHooks + `
if test -n "${PROMPT_COMMAND+_}"; then 
  export PROMPT_COMMAND="change_hermit_env; $PROMPT_COMMAND"
else
  export PROMPT_COMMAND="change_hermit_env"
fi

complete -o nospace -C "$HOME/bin/hermit" hermit
`

// Bash represent the Bash shell
type Bash struct{ posixMixin }

var _ Shell = &Bash{}

func (sh *Bash) Name() string { return "bash" } // nolint: golint

func (sh *Bash) ActivationScript(w io.Writer, config ActivationConfig) error { // nolint: golint
	err := posixActivationScriptTmpl.Execute(w, &posixActivationContext{
		ActivationConfig: config,
		EnvName:          filepath.Base(config.Root),
		Shell:            "bash",
	})
	return errors.WithStack(err)
}

func (sh *Bash) ActivationHooksInstallation() (path, script string, err error) { // nolint: golint
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	fileName := filepath.Join(home, ".bashrc")
	return fileName, `eval "$(test -x $HOME/bin/hermit && $HOME/bin/hermit shell-hooks --print --bash)"`, nil
}

func (sh *Bash) ActivationHooksCode() (script string, err error) { // nolint: golint
	return bashShellHooks, err
}
