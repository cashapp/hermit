package app

import (
	"os"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
	"github.com/pkg/errors"
)

type deactivateCmd struct{}

func (a *deactivateCmd) Run(env *hermit.Env, p *ui.UI) error {
	ops, err := env.EnvOps(p)
	if err != nil {
		return errors.WithStack(err)
	}
	sh, err := shell.Detect()
	if err != nil {
		return errors.WithStack(err)
	}
	environ := envars.Parse(os.Environ()).Revert(env.Root(), ops).Changed(true)
	return shell.DeactivateHermit(os.Stdout, sh, environ)
}
