package app

import (
	"fmt"
	"os"

	"github.com/pkg/errors"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type installCmd struct {
	Packages []manifest.GlobSelector `arg:"" optional:"" name:"package" help:"Packages to install (<name>[-<version>]). Version can be a glob to find the latest version with." predictor:"package"`
}

func (i *installCmd) Help() string {
	return `
Add the specified set of packages to the environment. If no packages are specified, all existing packages linked
into the environment will be downloaded and installed. Packages will be pinned to the version resolved at install time.
`
}

func (i *installCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	installed, err := env.ListInstalledReferences()
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs := map[string]*manifest.Package{}
	selectors := i.Packages

	err = env.Sync(l, false)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(selectors) == 0 {
		// Checking that all the packages are downloaded and unarchived
		for _, ref := range installed {
			task := l.Task(ref.String())
			pkg, err := env.Resolve(l, manifest.ExactSelector(ref), false)
			if err != nil {
				return errors.WithStack(err)
			}
			err = state.CacheAndUnpack(task, pkg)
			pkg.LogWarnings(l)
			task.Done()
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	// Check that we are not installing an already existing package
	for _, selector := range selectors {
		for _, ref := range installed {
			if selector.Matches(ref) {
				return errors.Errorf("%s cannot be installed as %s is already installed", selector, ref)
			}
		}
	}
	for i, search := range selectors {
		err := env.ResolveWithDeps(l, installed, selectors[i], pkgs)
		if err != nil {
			return errors.Wrap(err, search.String())
		}
	}
	changes := shell.NewChanges(envars.Parse(os.Environ()))
	w := l.WriterAt(ui.LevelInfo)
	defer w.Sync() // nolint
	for _, pkg := range pkgs {
		// Skip possible dependencies that have already been installed
		exists := false
		for _, ref := range installed {
			if ref.String() == pkg.Reference.String() {
				exists = true
				break
			}
		}
		if exists {
			continue
		}

		c, err := env.Install(l, pkg)
		if err != nil {
			return errors.WithStack(err)
		}
		messages, err := env.TriggerForPackage(l, manifest.EventInstall, pkg)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, message := range messages {
			fmt.Fprintln(w, message)
		}
		changes = changes.Merge(c)
		pkg.LogWarnings(l)
	}
	return nil
}
