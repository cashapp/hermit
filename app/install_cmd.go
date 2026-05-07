package app

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type installCmd struct {
	Packages    []manifest.GlobSelector `arg:"" optional:"" name:"package" help:"Packages to install (<name>[-<version>]). Version can be a glob to find the latest version with." predictor:"package"`
	Concurrency int                     `short:"j" help:"Number of parallel downloads." default:"0"`
}

func (i *installCmd) Help() string {
	return `
Add the specified set of packages to the environment. If no packages are specified, all existing packages linked
into the environment will be downloaded and installed. Packages will be pinned to the version resolved at install time.
`
}

func (i *installCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	concurrency := i.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	installed, err := env.ListInstalledReferences()
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs := map[string]*manifest.Package{}
	selectors := i.Packages

	err = env.Update(l, false)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(selectors) == 0 {
		// Resolve all packages first (sequential — resolver may hold shared state).
		type resolvedRef struct {
			pkg  *manifest.Package
			task *ui.Task
		}
		refs := make([]resolvedRef, 0, len(installed))
		for _, ref := range installed {
			task := l.Task(ref.String())
			pkg, err := env.Resolve(l, manifest.ExactSelector(ref), true)
			if err != nil {
				return errors.WithStack(err)
			}
			refs = append(refs, resolvedRef{pkg: pkg, task: task})
		}
		// Download and unpack all packages in parallel.
		g := errgroup.Group{}
		g.SetLimit(concurrency)
		for _, r := range refs {
			g.Go(func() error {
				err := state.CacheAndUnpack(r.task, r.pkg)
				r.pkg.LogWarnings(l)
				r.task.Done()
				return err
			})
		}
		if err := g.Wait(); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	var toBeInstalledSelectors []manifest.GlobSelector

	// Check that we are not installing an already existing package
	for _, selector := range selectors {
		for _, ref := range installed {
			if selector.Matches(ref) {
				l.Infof("skipping installation of %s as it is already installed", selector)

				continue
			}
		}

		toBeInstalledSelectors = append(toBeInstalledSelectors, selector)
	}

	for i, search := range toBeInstalledSelectors {
		err := env.ResolveWithDeps(l, installed, toBeInstalledSelectors[i], pkgs)
		if err != nil {
			return errors.Wrap(err, search.String())
		}
	}
	// Download and unpack all packages in parallel.
	g := errgroup.Group{}
	g.SetLimit(concurrency)
	for _, pkg := range pkgs {
		g.Go(func() error {
			task := l.Task(pkg.Reference.String())
			defer task.Done()
			return state.CacheAndUnpack(task, pkg)
		})
	}
	if err := g.Wait(); err != nil {
		return errors.WithStack(err)
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
