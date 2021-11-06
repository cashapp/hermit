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
	Packages []string `arg:"" optional:"" name:"package" help:"Packages to install (<name>[-<version>]). Version can be a glob to find the latest version with." predictor:"package"`
}

func (i *installCmd) Help() string {
	return `
Add the specified set of packages to the environment. If no packages are specified, all existing packages linked
into the environment will be downloaded and installed. Packages will be pinned to the version resolved at install time.
`
}

func (i *installCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs := map[string]*manifest.Package{}
	packages := i.Packages

	err = env.Sync(l, false)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(packages) == 0 {
		// Checking that all the packages are downloaded and unarchived
		for _, pkg := range installed {
			task := l.Task(pkg.Reference.String())
			err := state.CacheAndUnpack(task, pkg)
			pkg.LogWarnings(l)
			task.Done()
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	selectors := make([]manifest.Selector, len(packages))
	// Check that we are not installing an already existing package
	for i, search := range packages {
		selector, err := manifest.GlobSelector(search)
		if err != nil {
			return errors.WithStack(err)
		}
		selectors[i] = selector
		for _, ipkg := range installed {
			if selector.Matches(ipkg.Reference) {
				return errors.Errorf("%s cannot be installed as %s is already installed", selector.String(), ipkg.Reference)
			}
		}
	}
	for i, search := range packages {
		err := env.ResolveWithDeps(l, installed, selectors[i], pkgs)
		if err != nil {
			return errors.Wrap(err, search)
		}
	}
	changes := shell.NewChanges(envars.Parse(os.Environ()))
	w := l.WriterAt(ui.LevelInfo)
	defer w.Sync() // nolint
	for _, pkg := range pkgs {
		// Skip possible dependencies that have already been installed
		exists := false
		for _, ipkg := range installed {
			if ipkg.Reference.String() == pkg.Reference.String() {
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
