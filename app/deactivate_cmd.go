package app

import (
	"os"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
)

type deactivateCmd struct{}

func (a *deactivateCmd) Run(cli cliInterface, env *hermit.Env, p *ui.UI) error {
	ops, err := env.EnvOps(p)
	if err != nil {
		return errors.WithStack(err)
	}
	sh, err := shell.Detect(cli.getHermitBin())
	if err != nil {
		return errors.WithStack(err)
	}
	environ := envars.Parse(os.Environ()).Revert(env.Root(), ops).Changed(true)
	return shell.DeactivateHermit(os.Stdout, sh, environ)
}
