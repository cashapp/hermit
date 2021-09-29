package app

import (
	"os"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
	"github.com/pkg/errors"
)

type uninstallCmd struct {
	Packages []string `arg:"" help:"Packages to uninstall from this environment." predictor:"installed-package"`
}

func (u *uninstallCmd) Run(l *ui.UI, env *hermit.Env) error {
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	selectors := []manifest.Selector{}
	for _, pkg := range u.Packages {
		globs, err := manifest.GlobSelector(pkg)
		if err != nil {
			return errors.WithStack(err)
		}
		selectors = append(selectors, globs)
	}
	changes := shell.NewChanges(envars.Parse(os.Environ()))
next:
	for _, selector := range selectors {
		for _, pkg := range installed {
			if selector.Matches(pkg.Reference) {
				c, err := env.Uninstall(l, pkg)
				if err != nil {
					return errors.WithStack(err)
				}
				changes = changes.Merge(c)
				continue next
			}
		}
		return errors.Errorf("package %s is not installed", selector)
	}

	return nil
}
