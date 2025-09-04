package app

import (
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type updateCmd struct{}

func (s *updateCmd) Run(l *ui.UI, env *hermit.Env, state *state.State, cli cliInterface) error {
	srcs, err := state.Sources(l)
	if err != nil {
		return errors.WithStack(err)
	}
	// Update sources from either the env or default sources.
	if env != nil {
		err = env.Update(l, true)
	} else {
		err = srcs.Sync(l, true)
	}
	if err != nil {
		return errors.WithStack(err)
	}
	// Upgrade hermit if necessary
	err = maybeUpdateHermit(l, env, cli.getSelfUpdate(), true)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
