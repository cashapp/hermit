package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type bundleCmd struct {
	Dir      string                  `arg:"" help:"Directory in which to create the package bundle."`
	Packages []manifest.GlobSelector `arg:"" optional:"" help:"List of packages to include in the bundle."`
}

func (b *bundleCmd) Run(l *ui.UI, state *state.State, globalState GlobalState, cache *cache.Cache, env *hermit.Env) error {
	t := l.Task("bundle")
	defer t.Done()

	var err error
	b.Dir, err = filepath.Abs(b.Dir)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := os.MkdirAll(filepath.Join(b.Dir, "bin"), 0750); err != nil {
		return errors.Wrap(err, "failed to create bundle directory")
	}

	installed, err := env.ListInstalledReferences()
	if err != nil {
		return errors.WithStack(err)
	}

	state = state.
		WithPackageDir(b.Dir)
	env, err = env.
		WithState(state).
		WithBinDir(filepath.Join(b.Dir, "bin")).
		WithEnvDir(l, b.Dir)
	if err != nil {
		return errors.WithStack(err)
	}

	refs, err := b.matchPackages(installed)
	if err != nil {
		return errors.WithStack(err)
	}

	ops := envars.Ops{}
	for _, ref := range refs {
		task := l.Task(ref.String())
		pkg, err := env.Resolve(l, manifest.ExactSelector(ref), true)
		if err != nil {
			return errors.WithStack(err)
		}
		// Make the package mutable, as this is not a shared Hermit state dir.
		pkg.Mutable = true
		ops = append(ops, pkg.Env...)
		err = state.CacheAndUnpack(task, pkg)
		pkg.LogWarnings(l)
		task.Done()
		if err != nil {
			return errors.WithStack(err)
		}
		if err := b.mergeSymlinks(pkg); err != nil {
			return errors.WithStack(err)
		}
	}

	// To simulate applying the envars to a real set of envars, we set the values to the uninterpolated value, which will
	// get expanded by the shell.
	w, err := os.Create(filepath.Join(b.Dir, ".env"))
	if err != nil {
		return errors.WithStack(err)
	}
	vars := envars.Envars{
		"HERMIT_ENV": env.Root(),
		"HOME":       "${HOME}",
	}

	envrc := vars.Apply(env.Root(), ops).Changed(false)
	envrc["PATH"] = envrc["PATH"] + filepath.Join(b.Dir, "bin") + ":$PATH"
	for key, value := range envrc {
		envrc[key] = strings.ReplaceAll(value, b.Dir, "${PWD}")
	}
	for key, value := range envrc {
		fmt.Fprintf(w, "%s=%q\n", key, value)
	}
	l.Infof("Created exploded bundle:")
	l.Infof("  Root: %s", b.Dir)
	l.Infof("   Bin: %s", filepath.Join(b.Dir, "bin"))
	l.Infof("  .env: %s", filepath.Join(b.Dir, ".env"))
	for key, value := range envrc {
		l.Infof("        %s=%q", key, value)
	}
	return errors.WithStack(w.Close())
}

func (b *bundleCmd) matchPackages(installed []manifest.Reference) ([]manifest.Reference, error) {
	var pkgs []manifest.Reference
	if len(b.Packages) == 0 {
		pkgs = append(pkgs, installed...)
	} else {
	next:
		for _, glob := range b.Packages {
			for _, pkg := range installed {
				if glob.Matches(pkg) {
					pkgs = append(pkgs, pkg)
					continue next
				}
			}
			return nil, errors.Errorf("no installed package matches %q", glob)
		}
	}

	return pkgs, nil
}

func (b *bundleCmd) mergeSymlinks(pkg *manifest.Package) error {
	binaries, err := pkg.ResolveBinaries()
	if err != nil {
		return errors.WithStack(err)
	}
	for _, binary := range binaries {
		from, err := filepath.Rel(filepath.Join(b.Dir, "bin"), binary)
		if err != nil {
			return errors.WithStack(err)
		}
		to := filepath.Join(b.Dir, "bin", filepath.Base(binary))
		_ = os.Remove(to)
		if err := os.Symlink(from, to); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
