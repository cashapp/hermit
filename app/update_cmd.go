package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type updateCmd struct{}

func (s *updateCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	self, err := os.Executable()
	if err != nil {
		return errors.WithStack(err)
	}
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
	pkgRef := filepath.Base(filepath.Dir(self))
	if strings.HasPrefix(pkgRef, "hermit@") {
		pkg, err := state.Resolve(l, manifest.ExactSelector(manifest.ParseReference(pkgRef)))
		if err != nil {
			return errors.WithStack(err)
		}
		err = state.UpgradeChannel(l.Task(pkgRef), pkg)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
