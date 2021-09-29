package app

import (
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
	"github.com/pkg/errors"
)

type testCmd struct {
	Pkg []string `arg:"" required:"" help:"Run sanity tests for these packages."`
}

func (t *testCmd) Run(l *ui.UI, env *hermit.Env) error {
	for _, name := range t.Pkg {
		selector, err := manifest.GlobSelector(name)
		if err != nil {
			return errors.WithStack(err)
		}
		warnings, err := env.ValidateManifest(l, selector.Name())
		if err != nil {
			return errors.WithStack(err)
		}
		for _, warning := range warnings {
			l.Warnf("%s: %s", name, warning)
		}
		pkg, err := env.Resolve(l, selector, false)
		if errors.Is(err, manifest.ErrNoSource) {
			l.Warnf("No sources found for package %s on this architecture. Skipping the test", name)
			continue
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if err = env.Test(l, pkg); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
