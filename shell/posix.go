package shell

import (
	_ "embed" // Embedding files.
	"fmt"
	"html/template"
	"io"

	"github.com/pkg/errors"
	"github.com/cashapp/hermit/envars"
)

var (
	//go:embed files/activate.tmpl.sh
	posixActivationScript     string
	posixActivationScriptTmpl = template.Must(template.New("activation").Parse(posixActivationScript))
)

// Template context for activation script.
type posixActivationContext struct {
	Root    string
	EnvName string
	Shell   string
}

func (a posixActivationContext) Bash() bool { return a.Shell == "bash" }
func (a posixActivationContext) Zsh() bool  { return a.Shell == "zsh" }

// Functionality common to POSIX shells.
type posixMixin struct{}

func (sh *posixMixin) ApplyEnvars(w io.Writer, env envars.Envars) error {
	for key, value := range env {
		if value == "" {
			fmt.Fprintf(w, "unset %s\n", key)
		} else {
			fmt.Fprintf(w, "export %s=%s\n", key, Quote(value))
		}
	}
	return nil
}

func (sh *posixMixin) DeactivationScript(w io.Writer) error {
	_, err := fmt.Fprint(w, `
if test -n "${_HERMIT_OLD_PS1+_}"; then export PS1="${_HERMIT_OLD_PS1}"; unset _HERMIT_OLD_PS1; fi
unset ACTIVE_HERMIT
`)
	return errors.WithStack(err)
}
