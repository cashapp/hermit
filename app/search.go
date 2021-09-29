package app

import (
	"fmt"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/pkg/errors"
)

type searchCmd struct {
	Short      bool   `short:"s" help:"Short listing."`
	Constraint string `arg:"" help:"Package regex." optional:""`
}

func (s *searchCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	var (
		pkgs manifest.Packages
		err  error
	)
	if env != nil {
		err = env.Sync(l, false)
		if err != nil {
			return errors.WithStack(err)
		}
		pkgs, err = env.Search(l, s.Constraint)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		srcs, err := state.Sources(l)
		if err != nil {
			return errors.WithStack(err)
		}
		err = srcs.Sync(l, false)
		if err != nil {
			return errors.WithStack(err)
		}
		pkgs, err = state.Search(l, s.Constraint)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if s.Short {
		for _, pkg := range pkgs {
			fmt.Println(pkg)
		}
		return nil
	}
	listPackages(pkgs, true)
	return nil
}
