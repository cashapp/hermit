package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
)

type envCmd struct {
	Raw               bool   `short:"r" help:"Output raw values without shell quoting."`
	Ops               bool   `xor:"action" help:"Print the operations needed to manipulate the environment."`
	Activate          bool   `xor:"action" help:"Print the commands needed to set the environment to the activated state."`
	Deactivate        bool   `xor:"action" help:"Print the commands needed to reset the environment to the deactivated state."`
	DeactivateFromOps string `xor:"action" placeholder:"OPS" help:"Decodes the operations, and prints the shell commands to to reset the environment to the deactivated state."`
	Shell             string `short:"s" help:"Shell type."`
	Inherit           bool   `short:"i" help:"Inherit variables from parent environment."`
	Names             bool   `short:"n" help:"Show only names."`
	Unset             bool   `xor:"action" short:"u" help:"Unset the specified environment variable."`
	Name              string `arg:"" optional:"" help:"Name of the environment variable."`
	Value             string `arg:"" optional:"" help:"Value to set the variable to."`
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

	if e.Activate || e.Deactivate || e.Ops || e.DeactivateFromOps != "" {
		sh, err := e.resolveShell()
		if err != nil {
			return errors.WithStack(err)
		}
		ops, err := env.EnvOps(l)
		if err != nil {
			return errors.WithStack(err)
		}

		switch {
		case e.Activate:
			environ := envars.Parse(os.Environ()).Apply(env.Root(), ops).Changed(true)
			return errors.WithStack(sh.ApplyEnvars(os.Stdout, environ))

		case e.Ops:
			data, err := envars.MarshalOps(ops)
			if err != nil {
				return errors.Wrap(err, "failed to encode envar operations")
			}
			fmt.Println(string(data))

		case e.DeactivateFromOps != "":
			ops, err = envars.UnmarshalOps([]byte(e.DeactivateFromOps))
			if err != nil {
				return errors.Wrap(err, "failed to decode envar operations")
			}
			fallthrough

		case e.Deactivate:
			environ := envars.Parse(os.Environ()).Revert(env.Root(), ops).Changed(true)
			return errors.WithStack(sh.ApplyEnvars(os.Stdout, environ))
		}
		return nil
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

func (e *envCmd) resolveShell() (shell.Shell, error) {
	if e.Shell != "" {
		return shell.Resolve(e.Shell)
	}
	return shell.Detect()
}
