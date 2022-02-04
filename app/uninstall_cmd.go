package app

import (
	"fmt"
	"os"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
)

type uninstallCmd struct {
	Packages []manifest.GlobSelector `arg:"" help:"Packages to uninstall from this environment." predictor:"installed-package"`
}

func (u *uninstallCmd) Run(l *ui.UI, env *hermit.Env) error {
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	w := l.WriterAt(ui.LevelInfo)
	defer w.Sync() // nolint
	changes := shell.NewChanges(envars.Parse(os.Environ()))
next:
	for _, selector := range u.Packages {
		for _, pkg := range installed {
			if selector.Matches(pkg.Reference) {
				c, err := env.Uninstall(l, pkg)
				if err != nil {
					return errors.WithStack(err)
				}
				changes = changes.Merge(c)
				messages, err := env.TriggerForPackage(l, manifest.EventUninstall, pkg)
				if err != nil {
					return errors.WithStack(err)
				}
				for _, message := range messages {
					fmt.Fprintln(w, message)
				}
				continue next
			}
		}
		return errors.Errorf("package %s is not installed", selector)
	}

	return nil
}
