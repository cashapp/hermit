package shell

import (
	_ "embed"
	"fmt"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"io"
	"path/filepath"
	"text/template"
)

var (
	//go:embed files/fish_hooks.fish
	fishShellHooks string

	//go:embed files/activate.tmpl.fish
	fishActivationScript     string
	fishActivationScriptTmpl = template.Must(
		template.New("activation").
			Funcs(template.FuncMap{"Quote": Quote}).
			Parse(fishActivationScript),
	)
)

// Fish represent the Fish shell
type Fish struct{}

var _ Shell = &Fish{}

func (sh *Fish) Name() string { return "fish" } // nolint: golint

func (sh *Fish) ActivationScript(w io.Writer, config ActivationConfig) error { // nolint: golint
	err := fishActivationScriptTmpl.Execute(w, &fishActivationContext{
		ActivationConfig: config,
		EnvName:          filepath.Base(config.Root),
		Shell:            "fish",
	})
	return errors.WithStack(err)
}

func (sh *Fish) ActivationHooksInstallation() (path, script string, err error) { // nolint: golint
	return activationHooksInstallation(".config/fish/conf.d/hermit.fish", "fish")
}

func (sh *Fish) ActivationHooksCode() (script string, err error) { // nolint: golint
	return fishShellHooks, nil
}

func (sh *Fish) ApplyEnvars(w io.Writer, env envars.Envars) error {
	for key, value := range env {
		if value == "" {
			fmt.Fprintf(w, "set -e %s\n", key)
		} else {
			fmt.Fprintf(w, "set -gx %s %s\n", key, Quote(value))
		}
	}
	return nil
}

func (sh *Fish) DeactivationScript(w io.Writer) error {
	_, err := fmt.Fprint(w, `
set -e ACTIVE_HERMIT
`)
	return errors.WithStack(err)
}

// Template context for activation script.
type fishActivationContext struct {
	ActivationConfig
	EnvName string
	Shell   string
}
