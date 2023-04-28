package shell

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/system"
)

var zshShellHooks = `
precmd_functions+=(change_hermit_env)

# shellcheck disable=SC2154
if [[ -n ${_comps+x} ]]; then
  autoload -U +X bashcompinit && bashcompinit
  complete -o nospace -C "%s" hermit
fi
`

// Zsh represents the Zsh shell
type Zsh struct {
	posixMixin
	Bin string
}

func NewZsh(bin string) Shell {
	return &Zsh{Bin: bin}
}

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
	home, err := system.UserHomeDir()
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	return filepath.Join(home, ".zshrc"),
		fmt.Sprintf(`eval "$(test %[1]s && %[1]s shell-hooks --print --zsh --hermit-bin %[1]s)"`, sh.Bin),
		nil
}

func (sh *Zsh) ActivationHooksCode() (script string, err error) { // nolint: golint
	var buf bytes.Buffer
	if err := commonHooks(&buf, sh.Bin); err != nil {
		return "", errors.WithStack(err)
	}

	if _, err := fmt.Fprintf(&buf, zshShellHooks, sh.Bin); err != nil {
		return "", errors.WithStack(err)
	}

	return buf.String(), nil
}
