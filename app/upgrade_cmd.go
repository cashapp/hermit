package app

import (
	"os"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
)

type upgradeCmd struct {
	Packages []string `arg:"" optional:"" name:"package" help:"Packages to upgrade. If omitted, upgrades all installed packages."  predictor:"installed-package"`
}

func (g *upgradeCmd) Run(l *ui.UI, env *hermit.Env) error {
	err := env.Update(l, true)
	if err != nil {
		return errors.WithStack(err)
	}
	packages := []*manifest.Package{}
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	if g.Packages != nil {
		// check that the requested packages have been installed
		packageNames := map[string]*manifest.Package{}
		for _, pkg := range installed {
			packageNames[pkg.Reference.Name] = pkg
		}
		for _, name := range g.Packages {
			if packageNames[name] == nil {
				return errors.Errorf("no installed package '%s' found", name)
			}
			packages = append(packages, packageNames[name])
		}
	} else {
		packages = installed
	}

	changes := shell.NewChanges(envars.Parse(os.Environ()))

	// upgrade packages
	for _, pkg := range packages {
		c, err := env.Upgrade(l, pkg)
		if err != nil {
			return errors.WithStack(err)
		}
		changes = changes.Merge(c)
	}

	return nil
}
