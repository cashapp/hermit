package shell

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/system"
)

var bashShellHooks = `
if test -n "${PROMPT_COMMAND+_}"; then
  PROMPT_COMMAND="change_hermit_env; $PROMPT_COMMAND"
else
  PROMPT_COMMAND="change_hermit_env"
fi

complete -o nospace -C "%s" hermit
`

// Bash represent the Bash shell
type Bash struct {
	posixMixin
	Bin string
}

func NewBash(bin string) Shell {
	return &Bash{Bin: bin}
}

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
	home, err := system.UserHomeDir()
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	return filepath.Join(home, ".bashrc"),
		fmt.Sprintf(`eval "$(test -x %[1]s && %[1]s shell-hooks --print --bash --hermit-bin %[1]s)"`, sh.Bin),
		nil
}

func (sh *Bash) ActivationHooksCode() (script string, err error) { // nolint: golint
	var buf bytes.Buffer
	if err := commonHooks(&buf, sh.Bin); err != nil {
		return "", errors.WithStack(err)
	}

	if _, err := fmt.Fprintf(&buf, bashShellHooks, sh.Bin); err != nil {
		return "", errors.WithStack(err)
	}

	return buf.String(), nil
}
