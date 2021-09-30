package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
)

type envCmd struct {
	Raw        bool   `short:"r" help:"Output raw values without shell quoting."`
	Activate   bool   `xor:"envars" help:"Prints the commands needed to set the environment to the activated state"`
	Deactivate bool   `xor:"envars" help:"Prints the commands needed to reset the environment to the deactivated state"`
	Inherit    bool   `short:"i" help:"Inherit variables from parent environment."`
	Names      bool   `short:"n" help:"Show only names."`
	Unset      bool   `short:"u" help:"Unset the specified environment variable."`
	Name       string `arg:"" optional:"" help:"Name of the environment variable."`
	Value      string `arg:"" optional:"" help:"Value to set the variable to."`
}

func (e *envCmd) Help() string {
	return `
Without arguments the "env" command will display environment variables for the active Hermit environment.

Passing "<name>" will print the value for that environment variable.

Passing "<name> <value>" will set the value for an environment variable in the active Hermit environment."
	`
}

func (e *envCmd) Run(l *ui.UI, env *hermit.Env) error {
	// Special case for backwards compatibility.
	// TODO: Remove this at some point.
	if e.Name == "get" {
		e.Name = e.Value
		e.Value = ""
	}

	// Setting envar
	if e.Value != "" {
		return env.SetEnv(e.Name, e.Value)
	}

	if e.Unset {
		return env.DelEnv(e.Name)
	}

	if e.Activate {
		sh, err := shell.Detect()
		if err != nil {
			return errors.WithStack(err)
		}
		ops, err := env.EnvOps(l)
		if err != nil {
			return errors.WithStack(err)
		}
		environ := envars.Parse(os.Environ()).Apply(env.Root(), ops).Changed(true)
		return errors.WithStack(sh.ApplyEnvars(os.Stdout, environ))
	}

	if e.Deactivate {
		sh, err := shell.Detect()
		if err != nil {
			return errors.WithStack(err)
		}
		ops, err := env.EnvOps(l)
		if err != nil {
			return errors.WithStack(err)
		}
		environ := envars.Parse(os.Environ()).Revert(env.Root(), ops).Changed(true)
		return errors.WithStack(sh.ApplyEnvars(os.Stdout, environ))
	}

	// Display envars.
	envars, err := env.Envars(l, e.Inherit)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, v := range envars {
		parts := strings.SplitN(v, "=", 2)
		name := parts[0]
		value := parts[1]
		if e.Name != "" {
			if name == e.Name {
				fmt.Println(value)
				break
			}
			continue
		}
		if e.Names {
			fmt.Println(name)
		} else {
			if !e.Raw {
				value = shell.Quote(value)
			}
			fmt.Printf("%s=%s\n", name, value)
		}
	}
	return nil
}
